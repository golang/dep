// +build darwin dragonfly freebsd netbsd openbsd

package godirwalk

import (
	"reflect"
	"syscall"
	"unsafe"
)

func updateSliceDirentName(de *syscall.Dirent, nameSlice *[]byte, nameSliceHeader *reflect.SliceHeader) int {
	max := int(de.Namlen)
	nameSliceHeader.Cap = max
	nameSliceHeader.Len = max
	nameSliceHeader.Data = uintptr(unsafe.Pointer(&de.Name[0]))
	return max
}
