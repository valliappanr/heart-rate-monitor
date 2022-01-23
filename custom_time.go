package main

import (
	"fmt"
	"strings"
	"time"
)


type CustomTime struct {
	time.Time
}

const ctLayout = "2006-01-02 15:04:05"

func (ct *CustomTime) UnmarshalJSON(b []byte) (err error) {
	s := strings.Trim(string(b), "\"")
	if s == "null" {
		ct.Time = time.Time{}
		return
	}
	ct.Time, err = time.Parse(ctLayout, s)
	return
}

func (ct *CustomTime) MarshalJSON() ([]byte, error) {
	if ct.Time.UnixNano() == nilTime {
		return []byte("null"), nil
	}
	return []byte(fmt.Sprintf("\"%s\"", ct.Time.Format(ctLayout))), nil
}

func (ct *CustomTime) String() string {
	t := ct.Time
	return t.Format(time.RFC3339)
}

var nilTime = (time.Time{}).UnixNano()
func (ct *CustomTime) IsSet() bool {
	return ct.UnixNano() != nilTime
}
