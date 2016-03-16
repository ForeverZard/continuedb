package rafts

import (
	pb "continuedb/rafts/pb"
)

const noLimit = 0

func min(a, b uint64) uint64 {
	if a < b {
		return a
	} else {
		return b
	}
}

func max(a, b uint64) uint64 {
	if a < b {
		return b
	} else {
		return a
	}
}

func init() {
	defaultTickc = make(chan struct{})
}

// defaultTickc and Tick is the default logical clock for the raft instance.
var defaultTickc chan struct{}
func Tick() {
	defaultTickc <- struct{}{}
}

func limitSize(ents []*pb.Entry, maxsize uint64) []*pb.Entry {
	if len(ents) == 0 || maxsize == noLimit {
		return ents
	}
	size := ents[0].Size()
	var bound uint64
	for bound = 1; bound < uint64(len(ents)); bound++ {
		size += ents[bound].Size()
		if uint64(size) >= maxsize {
			break
		}
	}
	return ents[:bound]
}

func oneOf(mt pb.MessageType, types []pb.MessageType) bool {
	for _, t := range types {
		if t == mt {
			return true
		}
	}
	return false
}

func iconcat(a, b uint64) uint64 {
	var r, c uint64 = 10, b
	for c / 10 > 0 {
		r *= 10
		c %= 10
	}
	return a * r + b
}
