package configstore

import (
	"bytes"
	"sort"
)

// bytesTrim 去除首尾空白（避免把空白 JSON 当数据）。
func bytesTrim(b []byte) []byte {
	return bytes.TrimSpace(b)
}

// sortStrings 稳定升序排序字符串切片。
func sortStrings(s []string) {
	sort.Strings(s)
}
