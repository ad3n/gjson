//go:build darwin || linux

package gjson

import (
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

func TestIndexStringSpecialPageBounds(t *testing.T) {
	page := syscall.Getpagesize()
	buf, err := syscall.Mmap(-1, 0, page*3, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := syscall.Munmap(buf); err != nil {
			t.Error(err)
		}
	}()

	if err := syscall.Mprotect(buf[:page], syscall.PROT_NONE); err != nil {
		t.Fatal(err)
	}

	if err := syscall.Mprotect(buf[page*2:], syscall.PROT_NONE); err != nil {
		t.Fatal(err)
	}

	for i := page; i < page*2; i++ {
		buf[i] = 'a'
	}

	for size := 0; size <= 128; size++ {
		for _, start := range []int{page, page*2 - size} {
			s := unsafe.String(&buf[start], size)
			if got := indexStringSpecial(s); got != size {
				t.Fatalf("size=%d start=%d got=%d", size, start, got)
			}

			for pos := 0; pos < size; pos++ {
				for _, special := range []byte{'"', '\\'} {
					buf[start+pos] = special
					if got := indexStringSpecial(s); got != pos {
						t.Fatalf("size=%d start=%d pos=%d got=%d", size, start, pos, got)
					}

					buf[start+pos] = 'a'
				}
			}
		}
	}
}

func TestStringScanReadOnlyPages(t *testing.T) {
	page := syscall.Getpagesize()
	size := stringScanBlockSize*2 + page
	buf, err := syscall.Mmap(-1, 0, size+page*2, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := syscall.Munmap(buf); err != nil {
			t.Error(err)
		}
	}()

	for i := page; i < page+size; i++ {
		buf[i] = 'a'
	}

	buf[page] = '"'
	buf[page+size/2] = '\\'
	buf[page+size/2+1] = 'n'
	buf[page+size-1] = '"'
	if err := syscall.Mprotect(buf[:page], syscall.PROT_NONE); err != nil {
		t.Fatal(err)
	}

	if err := syscall.Mprotect(buf[page+size:], syscall.PROT_NONE); err != nil {
		t.Fatal(err)
	}

	if err := syscall.Mprotect(buf[page:page+size], syscall.PROT_READ); err != nil {
		t.Fatal(err)
	}

	for _, length := range []int{1, 15, 16, 17, 63, 64, 65, stringScanBlockSize - 1, stringScanBlockSize, stringScanBlockSize + 1, size} {
		input := unsafe.String(&buf[page+size-length], length)
		want := strings.IndexAny(input, "\"\\")
		if got := indexStringSpecial(input); got != want {
			t.Fatalf("length=%d got=%d want=%d", length, got, want)
		}

		assertParseString(t, input, 1)
	}
}
