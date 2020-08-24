package fat32

import (
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
)

const (
	// ShortDos71EBPB indicates that a DOS 7.1 EBPB is of the short 60-byte format
	shortDos71EBPB uint8 = 0x28
	// LongDos71EBPB indicates that a DOS 7.1 EBPB is of the long 79-byte format
	longDos71EBPB uint8 = 0x29
)

const (
	// FileSystemTypeFAT32 is the fixed string representation for the FAT32 filesystem type
	fileSystemTypeFAT32 string = "FAT32   "
)

// FatVersion is the version of the FAT filesystem
type fatVersion uint16

const (
	// FatVersion0 represents version 0 of FAT, the only acceptable version
	fatVersion0 fatVersion = 0
)

const (
	// FirstRemovableDrive is first removable drive
	firstRemovableDrive uint8 = 0x00
	// FirstFixedDrive is first fixed drive
	firstFixedDrive uint8 = 0x80
)

// Dos71EBPB is the DOS 7.1 Extended BIOS Parameter Block
type dos71EBPB struct {
	dos331BPB             *dos331BPB // Dos331BPB holds the embedded DOS 3.31 BIOS Parameter BLock
	sectorsPerFat         uint32     // SectorsPerFat is number of sectors per each table
	mirrorFlags           uint16     // MirrorFlags determines how FAT mirroring is done. If bit 7 is set, use bits 3-0 to determine active number of FATs (zero-based); if bit 7 is clear, use normal FAT mirroring
	version               fatVersion // Version is the version of the FAT, must be 0
	rootDirectoryCluster  uint32     // RootDirectoryCluster is the cluster containing the filesystem root directory, normally 2
	fsInformationSector   uint16     // FSInformationSector holds the sector which contains the primary DOS 7.1 Filesystem Information Cluster
	backupBootSector      uint16     // BackupBootSector holds the sector which contains the backup boot sector and following FSIS sectors
	bootFileName          [12]byte   // BootFileName is reserved and should be all 0x00
	driveNumber           uint8      // DriveNumber is the code for the relative position and type of this drive in the system
	reservedFlags         uint8      // ReservedFlags are flags used by the operating system and/or BIOS for various purposes, e.g. Windows NT CHKDSK status, OS/2 desired drive letter, etc.
	extendedBootSignature uint8      // ExtendedBootSignature contains the flag as to whether this is a short (60-byte) or long (79-byte) DOS 7.1 EBPB
	volumeSerialNumber    uint32     // VolumeSerialNumber usually generated by some form of date and time
	volumeLabel           string     // VolumeLabel, an arbitrary 11-byte string
	fileSystemType        string     // FileSystemType is the 8-byte string holding the name of the file system type
}

func (bpb *dos71EBPB) equal(a *dos71EBPB) bool {
	if (bpb == nil && a != nil) || (a == nil && bpb != nil) {
		return false
	}
	if bpb == nil && a == nil {
		return true
	}
	return bpb.dos331BPB.equal(a.dos331BPB) &&
		bpb.sectorsPerFat == a.sectorsPerFat &&
		bpb.mirrorFlags == a.mirrorFlags &&
		bpb.version == a.version &&
		bpb.rootDirectoryCluster == a.rootDirectoryCluster &&
		bpb.fsInformationSector == a.fsInformationSector &&
		bpb.backupBootSector == a.backupBootSector &&
		bpb.bootFileName == a.bootFileName &&
		bpb.driveNumber == a.driveNumber &&
		bpb.reservedFlags == a.reservedFlags &&
		bpb.extendedBootSignature == a.extendedBootSignature &&
		bpb.volumeSerialNumber == a.volumeSerialNumber &&
		bpb.volumeLabel == a.volumeLabel &&
		bpb.fileSystemType == a.fileSystemType
}

// Dos71EBPBFromBytes reads the FAT32 Extended BIOS Parameter Block from a slice of bytes
// these bytes are assumed to start at the beginning of the BPB, but can stretech for any length
// this is because the calling function should know where the EBPB starts, but not necessarily where it ends
func dos71EBPBFromBytes(b []byte) (*dos71EBPB, int, error) {
	if b == nil || (len(b) != 60 && len(b) != 79) {
		return nil, 0, errors.New("cannot read DOS 7.1 EBPB from invalid byte slice, must be precisely 60 or 79 bytes ")
	}
	bpb := dos71EBPB{}
	size := 0

	// extract the embedded DOS 3.31 BPB
	dos331bpb, err := dos331BPBFromBytes(b[0:25])
	if err != nil {
		return nil, 0, fmt.Errorf("Could not read embedded DOS 3.31 BPB: %v", err)
	}
	bpb.dos331BPB = dos331bpb

	bpb.sectorsPerFat = binary.LittleEndian.Uint32(b[25:29])
	bpb.mirrorFlags = binary.LittleEndian.Uint16(b[29:31])
	version := binary.LittleEndian.Uint16(b[31:33])
	if version != uint16(fatVersion0) {
		return nil, size, fmt.Errorf("Invalid FAT32 version found: %v", version)
	}
	bpb.version = fatVersion0
	bpb.rootDirectoryCluster = binary.LittleEndian.Uint32(b[33:37])
	bpb.fsInformationSector = binary.LittleEndian.Uint16(b[37:39])
	bpb.backupBootSector = binary.LittleEndian.Uint16(b[39:41])
	bootFileName := b[41:53]
	copy(bpb.bootFileName[:], bootFileName)
	bpb.driveNumber = uint8(b[53])
	bpb.reservedFlags = uint8(b[54])
	extendedSignature := uint8(b[55])
	bpb.extendedBootSignature = extendedSignature
	// is this a longer or shorter one
	bpb.volumeSerialNumber = binary.BigEndian.Uint32(b[56:60])

	switch extendedSignature {
	case shortDos71EBPB:
		size = 60
	case longDos71EBPB:
		size = 79
		// remove padding from each
		re := regexp.MustCompile("[ ]+$")
		bpb.volumeLabel = re.ReplaceAllString(string(b[60:71]), "")
		bpb.fileSystemType = re.ReplaceAllString(string(b[71:79]), "")
	default:
		return nil, size, fmt.Errorf("Unknown DOS 7.1 EBPB Signature: %v", extendedSignature)
	}

	return &bpb, size, nil
}

// ToBytes returns the Extended BIOS Parameter Block in a slice of bytes directly ready to
// write to disk
func (bpb *dos71EBPB) toBytes() ([]byte, error) {
	var b []byte
	// how many bytes is it? for extended, add the extended-specific stuff
	switch bpb.extendedBootSignature {
	case shortDos71EBPB:
		b = make([]byte, 60, 60)
	case longDos71EBPB:
		b = make([]byte, 79, 79)
		// do we have a valid volume label?
		label := bpb.volumeLabel
		if len(label) > 11 {
			return nil, fmt.Errorf("Invalid volume label: too long at %d characters, maximum is %d", len(label), 11)
		}
		labelR := []rune(label)
		if len(label) != len(labelR) {
			return nil, fmt.Errorf("Invalid volume label: non-ascii characters")
		}
		// pad with 0x20 = " "
		copy(b[60:71], []byte(fmt.Sprintf("%-11s", label)))
		// do we have a valid filesystem type?
		fstype := bpb.fileSystemType
		if len(fstype) > 8 {
			return nil, fmt.Errorf("Invalid filesystem type: too long at %d characters, maximum is %d", len(fstype), 8)
		}
		fstypeR := []rune(fstype)
		if len(fstype) != len(fstypeR) {
			return nil, fmt.Errorf("Invalid filesystem type: non-ascii characters")
		}
		// pad with 0x20 = " "
		copy(b[71:79], []byte(fmt.Sprintf("%-11s", fstype)))
	default:
		return nil, fmt.Errorf("Unknown DOS 7.1 EBPB Signature: %v", bpb.extendedBootSignature)
	}
	// fill in the common parts
	dos331Bytes, err := bpb.dos331BPB.toBytes()
	if err != nil {
		return nil, fmt.Errorf("Error converting embedded DOS 3.31 BPB to bytes: %v", err)
	}
	copy(b[0:25], dos331Bytes)
	binary.LittleEndian.PutUint32(b[25:29], bpb.sectorsPerFat)
	binary.LittleEndian.PutUint16(b[29:31], bpb.mirrorFlags)
	binary.LittleEndian.PutUint16(b[31:33], uint16(bpb.version))
	binary.LittleEndian.PutUint32(b[33:37], bpb.rootDirectoryCluster)
	binary.LittleEndian.PutUint16(b[37:39], bpb.fsInformationSector)
	binary.LittleEndian.PutUint16(b[39:41], bpb.backupBootSector)
	copy(b[41:53], bpb.bootFileName[:])
	b[53] = bpb.driveNumber
	b[54] = bpb.reservedFlags
	b[55] = bpb.extendedBootSignature
	binary.BigEndian.PutUint32(b[56:60], bpb.volumeSerialNumber)

	return b, nil
}
