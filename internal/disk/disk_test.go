/*
 * This file is part of then Oberon Disk Image Tool ("odit")
 * Copyright (C) 2025 Andreas Signer <asigner@gmail.com>
 *
 * odit is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * odit is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with Oberon Disk Image Tool.  If not, see <https://www.gnu.org/licenses/>.
 */
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
