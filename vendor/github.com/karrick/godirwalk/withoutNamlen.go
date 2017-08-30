// +build nacl linux solaris

package godirwalk

import (
	"bytes"
	"reflect"
	"syscall"
	"unsafe"
)

func updateSliceDirentName(de *syscall.Dirent, nameSlice *[]byte, nameSliceHeader *reflect.SliceHeader) int {
	// find the max possible name length
	max := int(uint64(de.Reclen) - uint64(unsafe.Offsetof(syscall.Dirent{}.Name)))

	// update slice header so slice points to name in syscall.Dirent struct
	nameSliceHeader.Cap = max
	nameSliceHeader.Len = max
	nameSliceHeader.Data = uintptr(unsafe.Pointer(&de.Name[0]))

	// find the actual name length by looking for the NULL byte
	if index := bytes.IndexByte(*nameSlice, 0); index >= 0 {
		nameSliceHeader.Cap = index
		nameSliceHeader.Len = index
		return index
	}

	return max
}
