package globalpb

// Key is the type of keys for kv continuedb.
type Key []byte

func MakeValueFromBytesAndTimestamp(val []byte, t *Timestamp) *Value {
	return &Value{
		RawBytes: val,
		Timestamp: t,
	}
}
