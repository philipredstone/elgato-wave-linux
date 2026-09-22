package usb

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

const (
	ioctlControl        = 0xC0185500 // USBDEVFS_CONTROL
	ioctlClaimInterface = 0x8004550F // USBDEVFS_CLAIMINTERFACE
	ioctlReset          = 0x00005514 // USBDEVFS_RESET

	requestTypeClassInterface = 0x21
	dirIn                     = 0x80

	controlTimeoutMs = 1000
)

// struct usbdevfs_ctrltransfer
type ctrlTransfer struct {
	requestType uint8
	request     uint8
	value       uint16
	index       uint16
	length      uint16
	timeout     uint32
	data        unsafe.Pointer
}

func (d *Device) ControlIn(request uint8, value, index uint16, length int) ([]byte, error) {
	buf := make([]byte, length)
	n, err := d.control(&ctrlTransfer{
		requestType: requestTypeClassInterface | dirIn,
		request:     request,
		value:       value,
		index:       index,
		length:      uint16(length),
		timeout:     controlTimeoutMs,
		data:        unsafe.Pointer(&buf[0]),
	})
	if err != nil {
		return nil, fmt.Errorf("control in %#04x: %w", value, err)
	}
	return buf[:n], nil
}

func (d *Device) ControlOut(request uint8, value, index uint16, data []byte) error {
	if len(data) == 0 {
		return errors.New("control out: empty payload")
	}
	_, err := d.control(&ctrlTransfer{
		requestType: requestTypeClassInterface,
		request:     request,
		value:       value,
		index:       index,
		length:      uint16(len(data)),
		timeout:     controlTimeoutMs,
		data:        unsafe.Pointer(&data[0]),
	})
	if err != nil {
		return fmt.Errorf("control out %#04x: %w", value, err)
	}
	return nil
}

func (d *Device) control(ct *ctrlTransfer) (int, error) {
	fd, err := syscall.Open(d.Node, syscall.O_RDWR, 0)
	if err != nil {
		return 0, err
	}
	defer syscall.Close(fd)

	// claim it ourselves or the kernel logs a line per transfer
	iface := uint32(ct.index & 0xff)
	ioctl(fd, ioctlClaimInterface, unsafe.Pointer(&iface))

	n, errno := ioctl(fd, ioctlControl, unsafe.Pointer(ct))
	if errno != 0 {
		return 0, errno
	}
	return n, nil
}

func (d *Device) Reset() error {
	fd, err := syscall.Open(d.Node, syscall.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	if _, errno := ioctl(fd, ioctlReset, nil); errno != 0 {
		return fmt.Errorf("usb reset: %w", errno)
	}
	return nil
}

func ioctl(fd int, req uintptr, arg unsafe.Pointer) (int, syscall.Errno) {
	n, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
	return int(n), errno
}

// EBUSY/EACCES is just the openwave GUI polling, not a dead device.
// treating it as one caused a reset storm once.
func IsFatal(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	switch errno {
	case syscall.ETIMEDOUT, syscall.EPIPE, syscall.EPROTO,
		syscall.EIO, syscall.ENODEV, syscall.ESHUTDOWN:
		return true
	}
	return false
}
