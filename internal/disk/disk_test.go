package disk

import (
	"os"
	"testing"
)

func TestAll(t *testing.T) {

	disk, err := Open("../../disk.img")
	if err != nil {
		pwd, _ := os.Getwd()
		t.Fatalf("Failed to open disk image: %v. pwd is %s", err, pwd)
	}
	defer disk.Close()

}
