# waved

Makes the Elgato Wave XLR actually usable on Linux. The mixer CLI (gain,
mute, headphone volume, low-Z) is a side effect of already having the
control protocol; the point is a daemon that keeps the interface alive.

## The problem

The Wave XLR runs capture and playback off one firmware clock. If the
playback endpoint gets opened before the capture endpoint (some app opens
the output first, a sample rate change, WirePlumber restarts, ...) the
firmware gets confused. What that looks like, in order of how much I hate
it:

- capture just stops delivering data
- capture delivers perfectly valid frames that are all zeros
- after a WirePlumber restart the streams hang off a different node
- ep0 dies, dmesg fills with `usb_set_interface failed (-110)`, and
  nothing short of pulling the cable brings it back

There is no quirk for `0fd9:007d` in `sound/usb/quirks.c`, so for now this
is worked around in userspace.

## Why not the WirePlumber fix

[wavexlr-on-linux-cfg](https://jmansar.github.io/wavexlr-on-linux-cfg/)
describes the same ordering problem and fixes it from inside WirePlumber:
`node.always-process = true` on the input node, or in the newer variant a
Lua script that keeps a virtual sink linked to the mic so capture never
goes idle.

That did not work for me (PipeWire 1.6, current WirePlumber). The
`always-process` variant is the one the page itself says stopped working
after updates, and I still ended up with a mic delivering zeros. Beyond
that, a WirePlumber rule can only influence the order of things
WirePlumber does. It doesn't notice when the firmware has already wedged,
it can't tell "all zeros" from "nothing plugged in", it can't reset the
device, and a WirePlumber restart throws the whole arrangement away. So
this became a separate daemon that watches the actual sample data and the
kernel log and recovers on its own.

## What it does

The daemon opens a capture stream first, then a silent playback stream,
and just holds both forever. That way the endpoints never get renegotiated
in the wrong order. It watches for the failure modes above and recovers
in steps: reopen the streams, restart WirePlumber, reset the USB port (max
once per 10 minutes), and finally nag you to replug.

It also puts gain/mute/hp/low-Z back after the device lost power, because
the firmware forgets at least the gain.

Single static Go binary, no cgo. USB via usbfs ioctls, PipeWire via
`pw-dump` and `pw-cat`.

## Install

Needs Go, PipeWire with WirePlumber, and a systemd user session.
`notify-send` is optional.

```sh
make install              # ~/.local/bin/waved + user unit
sudo make install-udev    # uaccess for the device
make enable
```

The daemon tails `journalctl -k` for the -110 messages. That only works
if your user can read the kernel log (wheel, adm or systemd-journal
group). Without that the hard-wedge detection is silently off; the rest
still works.

## Usage

```
waved status           device and daemon state
waved get              mixer settings
waved set gain 50      0..75 dB
waved set mute on
waved set hp -12.5     -128..0 dB
waved set low-z on
waved meters           input levels for two seconds
waved devices          supported devices
```

Config is `~/.config/waved/config`, see [config.example](config.example).
Saved mixer settings end up in `~/.local/state/waved/`.

## Other devices

| Device          | USB ID      |                              |
|-----------------|-------------|------------------------------|
| Elgato Wave XLR | `0fd9:007d` | stream guard + mixer, tested |
| Elgato Wave:3   | `0fd9:0070` | stream guard only, untested  |

The stream guard part is generic, it finds the PipeWire nodes through the
ALSA card. The mixer part needs the vendor protocol. Without it the
daemon can't tell a muted input from a wedge, so it only catches stalls
and wrong links.

To try something else: `device = vid:pid` in the config and add the ID to
the udev rule. To support it properly add it to `internal/device` and,
if you know the protocol, write a driver next to `internal/mixer/wavexlr`.

## Known issues / TODO

- No suspend/resume handling. Haven't seen it break yet, but the
  escalation ladder could in theory reach the USB reset while the
  device is still waking up.
- After a false hard-fault detection the daemon waits for a replug and
  never retries on its own. Restarting the unit works.
- `waved status` trusts a status file that goes stale if the daemon
  gets killed hard.
- Kernel quirk. The real fix is a `DEVICE_FLG` entry in `quirks.c`, probably
  `QUIRK_FLAG_FIXED_RATE` like the JBL Quantum810. Needs a usbmon capture
  of the wedge first. See NOTES.md.

## Credits

The control protocol, including the wIndex 0x3303 trick that keeps
snd-usb-audio bound, is from [OpenWave](https://github.com/rikkichy/openwave).
The capture-before-playback ordering was first written up in
[wavexlr-on-linux-cfg](https://github.com/jmansar/wavexlr-on-linux-cfg).

MIT.
