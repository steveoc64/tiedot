/* Document collection file. */
package chunkfile

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/steveoc64/tiedot/commonfile"
	"github.com/steveoc64/tiedot/tdlog"
)

const (
	/* Be aware that, changing the following constants will almost certainly require a number of test cases to be re-written. */

	COL_FILE_SIZE   = uint64(512 * 1024 * 1) // Size of collection data file
	DOC_MAX_ROOM    = uint64(512 * 1024 * 1) // Max single document size
	DOC_HEADER_SIZE = 1 + 10                 // Size of document header - validity (byte), document room (uint64)
	DOC_VALID       = byte(1)                // Document valid flag
	DOC_INVALID     = byte(0)                // Document invalid flag

	// Pre-compiled document padding (2048 spaces)
	PADDING = "                                                                                                                                " +
		"                                                                                                                                " +
		"                                                                                                                                " +
		"                                                                                                                                " +
		"                                                                                                                                " +
		"                                                                                                                                " +
		"                                                                                                                                " +
		"                                                                                                                                "
	LEN_PADDING = uint64(len(PADDING))
)

type ColFile struct {
	File *commonfile.File
}

// Open a collection file.
func OpenCol(name string) (*ColFile, error) {
	file, err := commonfile.Open(name, COL_FILE_SIZE)
	return &ColFile{File: file}, err
}

// Retrieve document data given its ID.
func (col *ColFile) Read(id uint64) []byte {
	if col.File.UsedSize < DOC_HEADER_SIZE || id >= col.File.UsedSize-DOC_HEADER_SIZE {
		return nil
	}
	if col.File.Buf[id] != DOC_VALID {
		return nil
	}
	if room, _ := binary.Uvarint(col.File.Buf[id+1 : id+11]); room > DOC_MAX_ROOM {
		return nil
	} else {
		docCopy := make([]byte, room)
		docEnd := id + DOC_HEADER_SIZE + room
		if docEnd >= col.File.Size {
			return nil
		}
		copy(docCopy, col.File.Buf[id+DOC_HEADER_SIZE:docEnd])
		return docCopy
	}
}

// Insert a document, return its ID.
func (col *ColFile) Insert(data []byte) (id uint64, err error) {
	len64 := uint64(len(data))
	room := len64 + len64
	if room > DOC_MAX_ROOM {
		err = errors.New(fmt.Sprintf("Document is too large"))
		return
	}
	// Keep track of new document ID and used space
	id = col.File.UsedSize
	if !col.File.CheckSize(DOC_HEADER_SIZE + room) {
		col.File.CheckSizeAndEnsure(DOC_HEADER_SIZE + room)
	}
	col.File.UsedSize = id + DOC_HEADER_SIZE + room
	// Make document header, then copy document data
	col.File.Buf[id] = 1
	binary.PutUvarint(col.File.Buf[id+1:id+DOC_HEADER_SIZE], room)
	paddingBegin := id + DOC_HEADER_SIZE + len64
	copy(col.File.Buf[id+DOC_HEADER_SIZE:paddingBegin], data)
	// Fill up padding space
	paddingEnd := id + DOC_HEADER_SIZE + room
	for segBegin := paddingBegin; segBegin < paddingEnd; segBegin += LEN_PADDING {
		segSize := LEN_PADDING
		segEnd := segBegin + LEN_PADDING

		if segEnd >= paddingEnd {
			segEnd = paddingEnd
			segSize = paddingEnd - segBegin
		}
		copy(col.File.Buf[segBegin:segEnd], PADDING[0:segSize])
	}
	return
}

// Update a document, return its new ID.
func (col *ColFile) Update(id uint64, data []byte) (newID uint64, err error) {
	len64 := uint64(len(data))
	if len64 > DOC_MAX_ROOM {
		err = errors.New(fmt.Sprintf("Updated document is too large"))
		return
	}
	if col.File.UsedSize < DOC_HEADER_SIZE || id >= col.File.UsedSize-DOC_HEADER_SIZE {
		err = errors.New(fmt.Sprintf("Document %d does not exist in %s", id, col.File.Name))
		return
	}
	if col.File.Buf[id] != DOC_VALID {
		err = errors.New(fmt.Sprintf("Document %d does not exist in %s", id, col.File.Name))
		return
	}
	if room, _ := binary.Uvarint(col.File.Buf[id+1 : id+11]); room > DOC_MAX_ROOM || id+room >= col.File.UsedSize {
		err = errors.New(fmt.Sprintf("Document %d does not exist in %s", id, col.File.Name))
		return
	} else {
		if len64 <= room {
			// There is enough room for the updated document
			// Overwrite document data
			paddingBegin := id + DOC_HEADER_SIZE + len64
			copy(col.File.Buf[id+DOC_HEADER_SIZE:paddingBegin], data)
			// Overwrite padding space
			paddingEnd := id + DOC_HEADER_SIZE + room
			for segBegin := paddingBegin; segBegin < paddingEnd; segBegin += LEN_PADDING {
				segSize := LEN_PADDING
				segEnd := segBegin + LEN_PADDING

				if segEnd >= paddingEnd {
					segEnd = paddingEnd
					segSize = paddingEnd - segBegin
				}
				copy(col.File.Buf[segBegin:segEnd], PADDING[0:segSize])
			}
			return id, nil
		}
		// There is not enough room for updated content, so delete the original document and re-insert
		col.Delete(id)
		return col.Insert(data)
	}
}

// Delete a document.
func (col *ColFile) Delete(id uint64) {
	if col.File.UsedSize < DOC_HEADER_SIZE || id >= col.File.UsedSize-DOC_HEADER_SIZE {
		return
	}
	if col.File.Buf[id] == DOC_VALID {
		col.File.Buf[id] = DOC_INVALID
	}
}

// Scan the entire data file, look for documents and invoke the function on each.
func (col *ColFile) ForAll(fun func(id uint64, doc []byte) bool) {
	addr := uint64(0)
	for {
		if col.File.UsedSize < DOC_HEADER_SIZE || addr >= col.File.UsedSize-DOC_HEADER_SIZE {
			break
		}
		// Read document header - validity and room
		validity := col.File.Buf[addr]
		room, _ := binary.Uvarint(col.File.Buf[addr+1 : addr+11])
		if validity != DOC_VALID && validity != DOC_INVALID || room > DOC_MAX_ROOM {
			// If the document does not contain valid header, skip it
			tdlog.Errorf("ERROR: The document at %d in %s is corrupted", addr, col.File.Name)
			// Move forward until we meet a valid document header
			for addr++; col.File.Buf[addr] != DOC_VALID && col.File.Buf[addr] != DOC_INVALID && addr < col.File.UsedSize-DOC_HEADER_SIZE; addr++ {
			}
			tdlog.Errorf("ERROR: Corrupted document skipped, now at %d", addr)
			continue
		}
		// If the function returns false, do not continue scanning
		if validity == DOC_VALID && !fun(addr, col.File.Buf[addr+DOC_HEADER_SIZE:addr+DOC_HEADER_SIZE+room]) {
			break
		}
		addr += DOC_HEADER_SIZE + room
	}
}
