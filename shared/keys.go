package shared

import (
	"strings"
	"strconv"
)

const (
	separator = ":"

	KeyNodeIdPrefix = "node"
	KeySentinel = "sentinel"
)

func MakeKey(components ...string) string {
	return strings.Join(components, separator)
}

func MakeNodeIdKey(nodeId uint64) string {
	return MakeKey(KeyNodeIdPrefix, strconv.FormatUint(nodeId, 10))
}
