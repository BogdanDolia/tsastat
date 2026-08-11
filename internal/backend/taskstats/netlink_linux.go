//go:build linux

package taskstats

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/BogdanDolia/tsastat/internal/model"
	"golang.org/x/sys/unix"
)

const (
	netlinkHeaderLength = 16
	genlHeaderLength    = 4
	netlinkAttrLength   = 4

	genlIDControl         = 0x10
	controlCommandGet     = 3
	controlAttrFamilyID   = 1
	controlAttrFamilyName = 2

	taskstatsCommandGet     = 1
	taskstatsVersion        = 1
	taskstatsTypePID        = 1
	taskstatsTypeStats      = 3
	taskstatsTypeAggregate  = 4
	taskstatsCommandAttrPID = 1

	netlinkRequestFlag  = 1
	netlinkErrorType    = 2
	netlinkDoneType     = 3
	netlinkAttrTypeMask = 0x3fff
)

type netlinkClient struct {
	fd       int
	portID   uint32
	familyID uint16
	sequence uint32
	mu       sync.Mutex
}

type netlinkAttribute struct {
	typeID uint16
	data   []byte
}

func newNetlinkClient(ctx context.Context) (statsClient, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_GENERIC)
	if err != nil {
		return nil, fmt.Errorf("socket: %w", err)
	}
	client := &netlinkClient{fd: fd}
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("bind: %w", err)
	}
	address, err := unix.Getsockname(fd)
	if err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("getsockname: %w", err)
	}
	netlinkAddress, ok := address.(*unix.SockaddrNetlink)
	if !ok {
		unix.Close(fd)
		return nil, fmt.Errorf("unexpected netlink socket address %T", address)
	}
	client.portID = netlinkAddress.Pid

	familyID, err := client.resolveFamily(ctx, "TASKSTATS")
	if err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("resolve TASKSTATS family: %w", err)
	}
	client.familyID = familyID
	return client, nil
}

func (c *netlinkClient) Stats(ctx context.Context, tid int, accountingEnabled, accountingEnabledKnown bool) (model.DelayCounters, error) {
	if tid <= 0 {
		return model.DelayCounters{}, fmt.Errorf("invalid tid %d", tid)
	}
	payload := make([]byte, 4)
	binary.NativeEndian.PutUint32(payload, uint32(tid))
	response, err := c.exchange(ctx, c.familyID, taskstatsCommandGet, taskstatsVersion, taskstatsCommandAttrPID, payload)
	if err != nil {
		if errors.Is(err, unix.ESRCH) {
			return model.DelayCounters{}, os.ErrNotExist
		}
		if errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) {
			return model.DelayCounters{}, fmt.Errorf("permission denied; TASKSTATS_CMD_GET requires CAP_NET_ADMIN: %w", err)
		}
		return model.DelayCounters{}, err
	}
	return decodeTaskstatsResponse(response, tid, accountingEnabled, accountingEnabledKnown)
}

func decodeTaskstatsResponse(response []byte, tid int, accountingEnabled, accountingEnabledKnown bool) (model.DelayCounters, error) {
	if len(response) < genlHeaderLength {
		return model.DelayCounters{}, fmt.Errorf("short TASKSTATS Generic Netlink response: %d bytes", len(response))
	}
	attributes, err := parseNetlinkAttributes(response[genlHeaderLength:])
	if err != nil {
		return model.DelayCounters{}, fmt.Errorf("parse TASKSTATS response attributes: %w", err)
	}
	for _, attribute := range attributes {
		if attribute.typeID != taskstatsTypeAggregate {
			continue
		}
		nested, err := parseNetlinkAttributes(attribute.data)
		if err != nil {
			return model.DelayCounters{}, fmt.Errorf("parse TASKSTATS aggregate attributes: %w", err)
		}
		var responsePID uint32
		var statsPayload []byte
		for _, child := range nested {
			switch child.typeID {
			case taskstatsTypePID:
				if len(child.data) >= 4 {
					responsePID = binary.NativeEndian.Uint32(child.data[:4])
				}
			case taskstatsTypeStats:
				statsPayload = child.data
			}
		}
		if responsePID != uint32(tid) || statsPayload == nil {
			continue
		}
		return decodeDelayCounters(statsPayload, accountingEnabled, accountingEnabledKnown)
	}
	return model.DelayCounters{}, fmt.Errorf("TASKSTATS response did not contain stats for tid %d", tid)
}

func (c *netlinkClient) Close() error {
	return unix.Close(c.fd)
}

func (c *netlinkClient) resolveFamily(ctx context.Context, name string) (uint16, error) {
	response, err := c.exchange(ctx, genlIDControl, controlCommandGet, 1, controlAttrFamilyName, append([]byte(name), 0))
	if err != nil {
		return 0, err
	}
	if len(response) < genlHeaderLength {
		return 0, fmt.Errorf("short Generic Netlink control response: %d bytes", len(response))
	}
	attributes, err := parseNetlinkAttributes(response[genlHeaderLength:])
	if err != nil {
		return 0, err
	}
	for _, attribute := range attributes {
		if attribute.typeID == controlAttrFamilyID && len(attribute.data) >= 2 {
			return binary.NativeEndian.Uint16(attribute.data[:2]), nil
		}
	}
	return 0, fmt.Errorf("Generic Netlink control response did not contain a family ID")
}

func (c *netlinkClient) exchange(ctx context.Context, messageType uint16, command, version uint8, attributeType uint16, attributeData []byte) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.sequence++
	sequence := c.sequence
	request := buildNetlinkRequest(messageType, command, version, sequence, c.portID, attributeType, attributeData)
	if err := unix.Sendto(c.fd, request, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return nil, fmt.Errorf("send netlink request: %w", err)
	}

	buffer := make([]byte, 64*1024)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for netlink response")
		}
		poll := []unix.PollFd{{Fd: int32(c.fd), Events: unix.POLLIN}}
		ready, err := unix.Poll(poll, 100)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return nil, fmt.Errorf("poll netlink response: %w", err)
		}
		if ready == 0 {
			continue
		}
		n, _, err := unix.Recvfrom(c.fd, buffer, unix.MSG_DONTWAIT)
		if err != nil {
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
				continue
			}
			return nil, fmt.Errorf("receive netlink response: %w", err)
		}
		payload, matched, err := parseNetlinkResponse(buffer[:n], sequence)
		if err != nil {
			return nil, err
		}
		if matched {
			return payload, nil
		}
	}
}

func buildNetlinkRequest(messageType uint16, command, version uint8, sequence, portID uint32, attributeType uint16, attributeData []byte) []byte {
	attributeSize := netlinkAttrLength + len(attributeData)
	totalSize := netlinkHeaderLength + genlHeaderLength + align4(attributeSize)
	message := make([]byte, totalSize)
	binary.NativeEndian.PutUint32(message[0:4], uint32(totalSize))
	binary.NativeEndian.PutUint16(message[4:6], messageType)
	binary.NativeEndian.PutUint16(message[6:8], netlinkRequestFlag)
	binary.NativeEndian.PutUint32(message[8:12], sequence)
	binary.NativeEndian.PutUint32(message[12:16], portID)
	message[16] = command
	message[17] = version
	attributeOffset := netlinkHeaderLength + genlHeaderLength
	binary.NativeEndian.PutUint16(message[attributeOffset:attributeOffset+2], uint16(attributeSize))
	binary.NativeEndian.PutUint16(message[attributeOffset+2:attributeOffset+4], attributeType)
	copy(message[attributeOffset+netlinkAttrLength:], attributeData)
	return message
}

func parseNetlinkResponse(message []byte, sequence uint32) ([]byte, bool, error) {
	for offset := 0; offset+netlinkHeaderLength <= len(message); {
		length := int(binary.NativeEndian.Uint32(message[offset : offset+4]))
		if length < netlinkHeaderLength || offset+length > len(message) {
			return nil, false, fmt.Errorf("invalid netlink message length %d", length)
		}
		typeID := binary.NativeEndian.Uint16(message[offset+4 : offset+6])
		messageSequence := binary.NativeEndian.Uint32(message[offset+8 : offset+12])
		payload := message[offset+netlinkHeaderLength : offset+length]
		if messageSequence == sequence {
			switch typeID {
			case netlinkErrorType:
				if len(payload) < 4 {
					return nil, false, fmt.Errorf("short netlink error response")
				}
				code := int32(binary.NativeEndian.Uint32(payload[:4]))
				if code == 0 {
					return nil, false, nil
				}
				if code < 0 {
					return nil, false, unix.Errno(-code)
				}
				return nil, false, fmt.Errorf("invalid positive netlink error code %d", code)
			case netlinkDoneType:
				return nil, false, fmt.Errorf("netlink response ended without data")
			default:
				return payload, true, nil
			}
		}
		offset += align4(length)
	}
	return nil, false, nil
}

func parseNetlinkAttributes(payload []byte) ([]netlinkAttribute, error) {
	attributes := make([]netlinkAttribute, 0, 4)
	for offset := 0; offset < len(payload); {
		if len(payload)-offset < netlinkAttrLength {
			if allZero(payload[offset:]) {
				break
			}
			return nil, fmt.Errorf("truncated netlink attribute header")
		}
		length := int(binary.NativeEndian.Uint16(payload[offset : offset+2]))
		if length < netlinkAttrLength || offset+length > len(payload) {
			return nil, fmt.Errorf("invalid netlink attribute length %d", length)
		}
		typeID := binary.NativeEndian.Uint16(payload[offset+2:offset+4]) & netlinkAttrTypeMask
		attributes = append(attributes, netlinkAttribute{
			typeID: typeID,
			data:   payload[offset+netlinkAttrLength : offset+length],
		})
		offset += align4(length)
	}
	return attributes, nil
}

func align4(value int) int {
	return (value + 3) &^ 3
}

func allZero(data []byte) bool {
	for _, value := range data {
		if value != 0 {
			return false
		}
	}
	return true
}
