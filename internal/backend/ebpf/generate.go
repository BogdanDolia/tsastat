package ebpf

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -Werror" -target bpfel,bpfeb -tags linux scheduler ./bpf/scheduler.c -- -I./bpf
