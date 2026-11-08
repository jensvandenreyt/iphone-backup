package backup

import (
	"errors"

	"howett.net/plist"
)

// keyedArchive is the NSKeyedArchiver layout of the MBFile blobs in Manifest.db.
type keyedArchive struct {
	Objects []any                `plist:"$objects"`
	Top     map[string]plist.UID `plist:"$top"`
}

func (ka keyedArchive) object(uid plist.UID) any {
	if int(uid) < len(ka.Objects) {
		return ka.Objects[uid]
	}
	return nil
}

// decodeMBFile fills in the size and encryption details of f from an MBFile archive.
func decodeMBFile(data []byte, f *File) error {
	var ka keyedArchive
	if _, err := plist.Unmarshal(data, &ka); err != nil {
		return err
	}
	root, ok := ka.object(ka.Top["root"]).(map[string]any)
	if !ok {
		return errors.New("mbfile: missing root object")
	}
	f.Size = toInt64(root["Size"])
	f.protectionClass = uint32(toInt64(root["ProtectionClass"]))
	uid, ok := root["EncryptionKey"].(plist.UID)
	if !ok {
		return nil
	}
	obj, _ := ka.object(uid).(map[string]any)
	key, _ := obj["NS.data"].([]byte)
	if len(key) < 4 {
		return errors.New("mbfile: invalid EncryptionKey")
	}
	// The first 4 bytes repeat the protection class.
	f.encryptionKey = key[4:]
	return nil
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case uint64:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	}
	return 0
}
