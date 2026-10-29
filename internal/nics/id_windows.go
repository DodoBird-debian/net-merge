package nics

import (
	"golang.org/x/sys/windows"
	"strings"
)

func adapterID(index int, fallback string) string {
	row := windows.MibIfRow2{InterfaceIndex: uint32(index)}
	if windows.GetIfEntry2Ex(windows.MibIfEntryNormal, &row) == nil {
		return strings.ToLower(row.InterfaceGuid.String())
	}
	return fallback
}
