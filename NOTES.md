# Notes

Loose collection of things I found out while debugging this. Mostly for
myself.

## The wedge

Device: Wave XLR, USB 0fd9:007d, UAC1, full speed. Sits on `usb 7-1.1`
here.

Capture and playback share one clock in the firmware. Open playback
first, then (re)open capture -> capture delivers exact zero samples. Not
"quiet", literally 0x0000 forever. The firmware meters (control request,
see below) still show signal in that state, which is how the daemon tells
"wedged" from "nothing plugged in".

Worse variant: ep0 stops answering. dmesg:

    usb 7-1.1: usb_set_interface failed (-110)

in a loop, `arecord -l` hangs, control transfers come back with -EIO.
A usbfs reset (USBDEVFS_RESET, same as usbreset(1)) does NOT fix this.
Only cutting VBUS does, i.e. pull the cable. Powered hubs with per-port
switching would probably work too, haven't tried.

Recovery order that works by hand: stop everything -> replug -> start
capture (any capture, pw-cat is fine) -> then playback. Doing it the
other way round reproduces the problem reliably.

## Things that went wrong

- First version pinged ep0 periodically and treated any error as "device
  dead". The openwave GUI polls the meters at 10 Hz, so we got EBUSY all
  the time, "diagnosed" a dead device, reset it, and the reset while
  snd-usb-audio was still attaching produced a REAL wedge. Then the next
  reset. Etc. Reset storm.
  Lesson: EBUSY/EACCES = contention. Only ETIMEDOUT/EPIPE/EPROTO/EIO/
  ENODEV mean anything. And no resets during attach, ever. Hence max one
  reset per 10 min.
- `systemctl --user restart wireplumber` fixes the stall variant on its
  own, no hardware reset. Full PCM close/reopen is enough. That's why
  it's step 2 of the ladder.
- After a WirePlumber restart, pw-cat with `--target` set to the XLR got
  moved to the default source (the virtual mic) anyway. The byte watchdog
  didn't notice because the virtual mic delivers zeros too. Fix is
  `node.dont-reconnect = true` on the stream, then pw-cat exits instead
  and the daemon notices. Might be worth a WirePlumber bug report.
- pw-cat playback from stdin needs `--raw`, otherwise it goes through
  libsndfile and says "Format not recognised".
- The mute byte has to be checked before deciding something is silent.
  Obvious in hindsight.
- Fast replug can show up as a stall rather than a disconnect (new devnum
  before the presence check ran). The daemon then happily saved the
  firmware defaults as if I'd turned the knob. Now: settings are only
  saved after the same value was read on two consecutive polls, and the
  restore is keyed on the usbfs node.

## Control protocol

All of this is from openwave's device.py, I just ported it.

    bmRequestType 0x21 / 0xa1 (class, interface)
    bRequest      0x05 set, 0x85 get
    wIndex        0x3303

wIndex: firmware only looks at the high byte (0x33). The kernel routes
on the low byte, interface 3, which no driver claims. So we can claim it
from usbfs and snd-usb-audio stays attached to the audio interfaces.
Sending to 0x3300 would need interface 0 and detach the audio driver.

wValue 0x0000: config block, 34 bytes, read and written as a whole.

    off  size
    0    2    gain, Q8.8 dB, 0..75
    4    1    mute
    9    2    headphone volume, Q8.8 dB, -128..0
    14   1    knob mode, 1 = gain, 2 = hp
    33   1    low-Z

wValue 0x0001: meters, 10 bytes, two u32 LE (L, R). Rest unknown.

wValue 0x000a: device info, 51 bytes. Firmware version at [6..8],
serial as NUL padded string at [27..47].

Everything else in the config block: no idea, left as is on write.

## Kernel side

No entry for 0fd9:007d in sound/usb/quirks.c. Candidates:

- `QUIRK_FLAG_FIXED_RATE`, like the JBL Quantum810 entry. Would stop the
  rate renegotiation that seems to trigger the wedge.
- longer term a mixer quirk driver along the lines of mixer_scarlett2.c
  so gain/mute show up as ALSA controls.

Need a usbmon capture of a wedge happening to have something to show
upstream. Not done yet.
