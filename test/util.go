package test

import (
	"bytes"
	"encoding/json"
)

func AreEqualJSON(s1, s2 string) (bool, error) {
	var o1 interface{}
	var o2 interface{}
	var err error

	if err = json.Unmarshal([]byte(s1), &o1); err != nil {
		return false, err
	}
	if err = json.Unmarshal([]byte(s2), &o2); err != nil {
		return false, err
	}
	b1, _ := json.Marshal(o1)
	b2, _ := json.Marshal(o2)
	return bytes.Equal(b1, b2), nil
}
