package backup

import (
	"crypto/pbkdf2"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
)

// wrapPasscode marks class keys that are wrapped with the key derived from the backup password.
const wrapPasscode = 2

type classKey struct {
	class uint32
	wrap  uint32
	wpky  []byte
	key   []byte
}

// keybag is the parsed BackupKeyBag from Manifest.plist.
type keybag struct {
	salt, dpsl []byte
	iter, dpic int
	classes    map[uint32]*classKey
}

// parseKeybag reads the TLV-encoded keybag: a header followed by one record per protection class,
// each record starting with a UUID tag.
func parseKeybag(data []byte) (*keybag, error) {
	kb := &keybag{classes: map[uint32]*classKey{}}
	var cur *classKey
	seenUUID := false
	for len(data) >= 8 {
		tag := string(data[:4])
		n := binary.BigEndian.Uint32(data[4:8])
		if uint64(n) > uint64(len(data)-8) {
			return nil, errors.New("keybag: truncated")
		}
		val := data[8 : 8+n]
		data = data[8+n:]
		switch tag {
		case "UUID":
			if !seenUUID {
				seenUUID = true
				continue
			}
			kb.add(cur)
			cur = &classKey{}
		case "CLAS", "WRAP", "WPKY":
			if cur != nil {
				cur.set(tag, val)
			}
		case "SALT":
			kb.salt = val
		case "ITER":
			kb.iter = int(u32(val))
		case "DPSL":
			kb.dpsl = val
		case "DPIC":
			kb.dpic = int(u32(val))
		}
	}
	kb.add(cur)
	if len(kb.salt) == 0 || kb.iter == 0 {
		return nil, errors.New("keybag: missing SALT or ITER")
	}
	return kb, nil
}

func (kb *keybag) add(ck *classKey) {
	if ck != nil {
		kb.classes[ck.class] = ck
	}
}

func (ck *classKey) set(tag string, val []byte) {
	switch tag {
	case "CLAS":
		ck.class = u32(val)
	case "WRAP":
		ck.wrap = u32(val)
	case "WPKY":
		ck.wpky = val
	}
}

func u32(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return binary.BigEndian.Uint32(b)
}

// unlock derives the passcode key from password and unwraps all class keys.
// It returns ErrWrongPassword when any unwrap fails its integrity check.
func (kb *keybag) unlock(password string) error {
	pw := []byte(password)
	if kb.dpic > 0 {
		k, err := pbkdf2.Key(sha256.New, password, kb.dpsl, kb.dpic, 32)
		if err != nil {
			return err
		}
		pw = k
	}
	passcodeKey, err := pbkdf2.Key(sha1.New, string(pw), kb.salt, kb.iter, 32)
	if err != nil {
		return err
	}
	keys := map[uint32][]byte{}
	for class, ck := range kb.classes {
		if ck.wrap&wrapPasscode == 0 {
			continue
		}
		k, err := aesUnwrap(passcodeKey, ck.wpky)
		if err != nil {
			return ErrWrongPassword
		}
		keys[class] = k
	}
	for class, k := range keys {
		kb.classes[class].key = k
	}
	return nil
}

func (kb *keybag) unwrapKey(class uint32, wrapped []byte) ([]byte, error) {
	ck, ok := kb.classes[class]
	if !ok || ck.key == nil {
		return nil, fmt.Errorf("no key for protection class %d", class)
	}
	return aesUnwrap(ck.key, wrapped)
}
