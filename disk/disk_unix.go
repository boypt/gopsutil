// +build freebsd linux darwin

package disk

import (
	"context"
	"reflect"
	"strconv"

	"golang.org/x/sys/unix"
)

// Usage returns a file system usage. path is a filesystem path such
// as "/", not device file path like "/dev/vda1".  If you want to use
// a return value of disk.Partitions, use "Mountpoint" not "Device".
func Usage(path string) (*UsageStat, error) {
	return UsageWithContext(context.Background(), path)
}

func UsageWithContext(ctx context.Context, path string) (*UsageStat, error) {
	stat := unix.Statfs_t{}
	err := unix.Statfs(path, &stat)
	if err != nil {
		return nil, err
	}
	// When container is run in MacOS, `bsize` obtained by `statfs` syscall is not the fundamental block size,
	// but the `iosize` (optimal transfer block size) instead, it's usually 1024 times larger than the `bsize`.
	// for example `4096 * 1024`. To get the correct block size, we should use `frsize`. But `frsize` isn't
	// guaranteed to be supported everywhere, so we need to check whether it's supported before use it.
	// For more details, please refer to: https://github.com/docker/for-mac/issues/2136
	bSize := uint64(stat.Bsize)
	field := reflect.ValueOf(&stat).Elem().FieldByName("Frsize")
	if field.IsValid() {
		if field.Kind() == reflect.Uint64 {
			bSize = field.Uint()
		} else {
			bSize = uint64(field.Int())
		}
	}

	ret := &UsageStat{
		Path:        unescapeFstab(path),
		Fstype:      getFsType(stat),
		Total:       (uint64(stat.Blocks) * bSize),
		Free:        (uint64(stat.Bavail) * bSize),
		InodesTotal: (uint64(stat.Files)),
		InodesFree:  (uint64(stat.Ffree)),
	}

	// if could not get InodesTotal, return empty
	if ret.InodesTotal < ret.InodesFree {
		return ret, nil
	}

	ret.InodesUsed = (ret.InodesTotal - ret.InodesFree)
	ret.Used = (uint64(stat.Blocks) - uint64(stat.Bfree)) * uint64(bsize)

	if ret.InodesTotal == 0 {
		ret.InodesUsedPercent = 0
	} else {
		ret.InodesUsedPercent = (float64(ret.InodesUsed) / float64(ret.InodesTotal)) * 100.0
	}

	if (ret.Used + ret.Free) == 0 {
		ret.UsedPercent = 0
	} else {
		// We don't use ret.Total to calculate percent.
		// see https://github.com/shirou/gopsutil/issues/562
		ret.UsedPercent = (float64(ret.Used) / float64(ret.Used+ret.Free)) * 100.0
	}

	return ret, nil
}

// Unescape escaped octal chars (like space 040, ampersand 046 and backslash 134) to their real value in fstab fields issue#555
func unescapeFstab(path string) string {
	escaped, err := strconv.Unquote(`"` + path + `"`)
	if err != nil {
		return path
	}
	return escaped
}
