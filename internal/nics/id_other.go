//go:build !windows

package nics

func adapterID(_ int, name string) string { return name }
