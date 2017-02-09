package i2c

import (
	"time"

	"gobot.io/x/gobot"
)

const wiichuckAddress = 0x52

type WiichuckDriver struct {
	name       string
	connector  I2cConnector
	connection I2cConnection
	I2cBusser
	interval  time.Duration
	pauseTime time.Duration
	gobot.Eventer
	joystick map[string]float64
	data     map[string]float64
}

// NewWiichuckDriver creates a WiichuckDriver with specified i2c interface.
//
// It adds the following events:
//	"z"- Gets triggered every interval amount of time if the z button is pressed
//	"c" - Gets triggered every interval amount of time if the c button is pressed
//	"joystick" - Gets triggered every "interval" amount of time if a joystick event occurred, you can access values x, y
//	"error" - Gets triggered whenever the WiichuckDriver encounters an error
func NewWiichuckDriver(a I2cConnector, options ...func(I2cBusser)) *WiichuckDriver {
	w := &WiichuckDriver{
		name:      gobot.DefaultName("Wiichuck"),
		connector: a,
		I2cBusser: NewI2cBusser(),
		interval:  10 * time.Millisecond,
		pauseTime: 1 * time.Millisecond,
		Eventer:   gobot.NewEventer(),
		joystick: map[string]float64{
			"sy_origin": -1,
			"sx_origin": -1,
		},
		data: map[string]float64{
			"sx": 0,
			"sy": 0,
			"z":  0,
			"c":  0,
		},
	}

	for _, option := range options {
		option(w)
	}

	w.AddEvent(Z)
	w.AddEvent(C)
	w.AddEvent(Joystick)
	w.AddEvent(Error)

	return w
}
func (w *WiichuckDriver) Name() string                 { return w.name }
func (w *WiichuckDriver) SetName(n string)             { w.name = n }
func (w *WiichuckDriver) Connection() gobot.Connection { return w.connector.(gobot.Connection) }

// Start initilizes i2c and reads from adaptor
// using specified interval to update with new value
func (w *WiichuckDriver) Start() (err error) {
	if w.GetBus() == BusNotInitialized {
		w.Bus(w.connector.I2cGetDefaultBus())
	}
	bus := w.GetBus()

	w.connection, err = w.connector.I2cGetConnection(wiichuckAddress, bus)
	if err != nil {
		return err
	}

	go func() {
		for {
			if _, err := w.connection.Write([]byte{0x40, 0x00}); err != nil {
				w.Publish(w.Event(Error), err)
				continue
			}
			time.Sleep(w.pauseTime)
			if _, err := w.connection.Write([]byte{0x00}); err != nil {
				w.Publish(w.Event(Error), err)
				continue
			}
			time.Sleep(w.pauseTime)
			newValue := make([]byte, 6)
			bytesRead, err := w.connection.Read(newValue)
			if err != nil {
				w.Publish(w.Event(Error), err)
				continue
			}
			if bytesRead == 6 {
				if err = w.update(newValue); err != nil {
					w.Publish(w.Event(Error), err)
					continue
				}
			}
			time.Sleep(w.interval)
		}
	}()
	return
}

// Halt returns true if driver is halted successfully
func (w *WiichuckDriver) Halt() (err error) { return }

// update parses value to update buttons and joystick.
// If value is encrypted, warning message is printed
func (w *WiichuckDriver) update(value []byte) (err error) {
	if w.isEncrypted(value) {
		return ErrEncryptedBytes
	} else {
		w.parse(value)
		w.adjustOrigins()
		w.updateButtons()
		w.updateJoystick()
	}
	return
}

// setJoystickDefaultValue sets default value if value is -1
func (w *WiichuckDriver) setJoystickDefaultValue(joystickAxis string, defaultValue float64) {
	if w.joystick[joystickAxis] == -1 {
		w.joystick[joystickAxis] = defaultValue
	}
}

// calculateJoystickValue returns distance between axis and origin
func (w *WiichuckDriver) calculateJoystickValue(axis float64, origin float64) float64 {
	return float64(axis - origin)
}

// isEncrypted returns true if value is encrypted
func (w *WiichuckDriver) isEncrypted(value []byte) bool {
	if value[0] == value[1] && value[2] == value[3] && value[4] == value[5] {
		return true
	}
	return false
}

// decode removes encoding from `x` byte
func (w *WiichuckDriver) decode(x byte) float64 {
	return float64((x ^ 0x17) + 0x17)
}

// adjustOrigins sets sy_origin and sx_origin with values from data
func (w *WiichuckDriver) adjustOrigins() {
	w.setJoystickDefaultValue("sy_origin", w.data["sy"])
	w.setJoystickDefaultValue("sx_origin", w.data["sx"])
}

// updateButtons publishes "c" and "x" events if present in data
func (w *WiichuckDriver) updateButtons() {
	if w.data["c"] == 0 {
		w.Publish(w.Event(C), true)
	}
	if w.data["z"] == 0 {
		w.Publish(w.Event(Z), true)
	}
}

// updateJoystick publishes event with current x and y values for joystick
func (w *WiichuckDriver) updateJoystick() {
	w.Publish(w.Event(Joystick), map[string]float64{
		"x": w.calculateJoystickValue(w.data["sx"], w.joystick["sx_origin"]),
		"y": w.calculateJoystickValue(w.data["sy"], w.joystick["sy_origin"]),
	})
}

// parse sets driver values based on parsed value
func (w *WiichuckDriver) parse(value []byte) {
	w.data["sx"] = w.decode(value[0])
	w.data["sy"] = w.decode(value[1])
	w.data["z"] = float64(uint8(w.decode(value[5])) & 0x01)
	w.data["c"] = float64(uint8(w.decode(value[5])) & 0x02)
}
