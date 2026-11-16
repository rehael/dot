// Package kv reads and writes line-format files.
package kv

type Entry struct{ Key, Value string }

func Parse(data []byte) []Entry { panic("not implemented") }

func Get(entries []Entry, key string) (string, bool) { panic("not implemented") }

func All(entries []Entry, key string) []string { panic("not implemented") }

func Set(data []byte, key, value string) []byte { panic("not implemented") }

func Append(data []byte, key, value string) []byte { panic("not implemented") }
