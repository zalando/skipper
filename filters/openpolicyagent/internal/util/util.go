package util

import (
	"crypto/rand"
	"fmt"
	"io"

	"github.com/zalando/skipper/filters"
)

func Uuid4() (string, error) {
	bs := make([]byte, 16)
	_, err := io.ReadFull(rand.Reader, bs)
	if err != nil {
		return "", err
	}
	bs[8] = bs[8]&^0xc0 | 0x80
	bs[6] = bs[6]&^0xf0 | 0x40
	return fmt.Sprintf("%x-%x-%x-%x-%x", bs[0:4], bs[4:6], bs[6:8], bs[8:10], bs[10:]), nil
}

func GetStrings(args []interface{}) ([]string, error) {
	s := make([]string, len(args))
	var ok bool
	for i, a := range args {
		s[i], ok = a.(string)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}
	}

	return s, nil
}
