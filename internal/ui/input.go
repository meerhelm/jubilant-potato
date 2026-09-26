package ui

import (
	"encoding/hex"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/meerhelm/jubilant-potato/internal/config"
)

// Action is a logical button press named after the printed button labels:
// A confirms, B goes back.
type Action int

const (
	None Action = iota
	Up
	Down
	Left
	Right
	A
	B
	X
	Y
	L1
	R1
	Select
	Start
	Menu
)

var actionNames = map[string]Action{
	"up": Up, "down": Down, "left": Left, "right": Right, "a": A, "b": B, "x": X, "y": Y,
	"l1": L1, "r1": R1, "select": Select, "start": Start, "menu": Menu,
}

const (
	repeatDelay    = 350 * time.Millisecond
	repeatInterval = 55 * time.Millisecond
	axisPress      = 16000
	axisRelease    = 8000
)

type axisKey struct {
	joy  sdl.JoystickID
	axis uint8
}

// Input turns SDL events into Actions with auto-repeat for held buttons.
type Input struct {
	swapAB     bool
	joyButtons map[uint8]Action

	controllers map[sdl.JoystickID]*sdl.GameController
	joysticks   map[sdl.JoystickID]*sdl.Joystick

	// capture, when set, receives every event instead of action mapping
	// (the button setup wizard).
	capture func(sdl.Event)

	held    map[Action]time.Time // action -> next repeat time
	axisDir map[axisKey]Action
	hatDir  map[sdl.JoystickID]uint8
}

func newInput(swapAB bool, joyButtons map[string]string) *Input {
	in := &Input{
		swapAB:      swapAB,
		joyButtons:  map[uint8]Action{},
		controllers: map[sdl.JoystickID]*sdl.GameController{},
		joysticks:   map[sdl.JoystickID]*sdl.Joystick{},
		held:        map[Action]time.Time{},
		axisDir:     map[axisKey]Action{},
		hatDir:      map[sdl.JoystickID]uint8{},
	}
	for k, v := range joyButtons {
		idx, err := strconv.Atoi(k)
		if a, ok := actionNames[strings.ToLower(v)]; ok && err == nil {
			in.joyButtons[uint8(idx)] = a
		}
	}
	return in
}

// debugInput logs every button event when $POTATO_DEBUG is set.
var debugInput = os.Getenv("POTATO_DEBUG") != ""

// Handle processes one event and returns the actions it triggers.
func (in *Input) Handle(ev sdl.Event) []Action {
	if in.capture != nil {
		switch e := ev.(type) {
		case *sdl.JoyDeviceAddedEvent:
			in.open(int(e.Which))
		case *sdl.JoyDeviceRemovedEvent:
			in.close(e.Which)
		default:
			in.held = map[Action]time.Time{}
			in.capture(ev)
		}
		return nil
	}
	acts := in.handle(ev)
	if debugInput {
		switch e := ev.(type) {
		case *sdl.ControllerButtonEvent:
			log.Printf("input: controller %d button %d state %d -> %v", e.Which, e.Button, e.State, acts)
		case *sdl.JoyButtonEvent:
			log.Printf("input: joystick %d button %d state %d -> %v", e.Which, e.Button, e.State, acts)
		case *sdl.JoyHatEvent:
			log.Printf("input: joystick %d hat %d value %d -> %v", e.Which, e.Hat, e.Value, acts)
		case *sdl.KeyboardEvent:
			log.Printf("input: key %d state %d -> %v", e.Keysym.Sym, e.State, acts)
		}
	}
	return acts
}

func (in *Input) handle(ev sdl.Event) []Action {
	switch e := ev.(type) {
	case *sdl.JoyDeviceAddedEvent:
		in.open(int(e.Which))
	case *sdl.JoyDeviceRemovedEvent:
		in.close(e.Which)

	case *sdl.KeyboardEvent:
		if e.Repeat != 0 {
			return nil
		}
		return in.set(keyAction(e.Keysym.Sym), e.Type == sdl.KEYDOWN)

	case *sdl.ControllerButtonEvent:
		return in.set(in.controllerButton(e.Button), e.Type == sdl.CONTROLLERBUTTONDOWN)
	case *sdl.ControllerAxisEvent:
		switch e.Axis {
		case sdl.CONTROLLER_AXIS_LEFTX:
			return in.axis(axisKey{e.Which, e.Axis}, e.Value, Left, Right)
		case sdl.CONTROLLER_AXIS_LEFTY:
			return in.axis(axisKey{e.Which, e.Axis}, e.Value, Up, Down)
		}

	// Raw joystick events are only used for pads without a controller mapping.
	case *sdl.JoyButtonEvent:
		if _, ok := in.controllers[e.Which]; !ok {
			return in.set(in.joyButtons[e.Button], e.Type == sdl.JOYBUTTONDOWN)
		}
	case *sdl.JoyHatEvent:
		if _, ok := in.controllers[e.Which]; !ok {
			return in.hat(e.Which, e.Value)
		}
	case *sdl.JoyAxisEvent:
		if _, ok := in.controllers[e.Which]; !ok && e.Axis < 2 {
			if e.Axis == 0 {
				return in.axis(axisKey{e.Which, e.Axis}, e.Value, Left, Right)
			}
			return in.axis(axisKey{e.Which, e.Axis}, e.Value, Up, Down)
		}
	}
	return nil
}

// Repeats returns auto-repeat actions for buttons that are still held.
func (in *Input) Repeats(now time.Time) []Action {
	var out []Action
	for a, next := range in.held {
		if now.After(next) {
			out = append(out, a)
			in.held[a] = now.Add(repeatInterval)
		}
	}
	return out
}

func (in *Input) set(a Action, down bool) []Action {
	if a == None {
		return nil
	}
	if !down {
		delete(in.held, a)
		return nil
	}
	if repeats(a) {
		in.held[a] = time.Now().Add(repeatDelay)
	}
	return []Action{a}
}

func repeats(a Action) bool {
	switch a {
	case Up, Down, Left, Right, L1, R1:
		return true
	}
	return false
}

func (in *Input) axis(k axisKey, v int16, neg, pos Action) []Action {
	prev := in.axisDir[k]
	cur := prev
	switch {
	case v <= -axisPress:
		cur = neg
	case v >= axisPress:
		cur = pos
	case v > -axisRelease && v < axisRelease:
		cur = None
	}
	if cur == prev {
		return nil
	}
	in.axisDir[k] = cur
	in.set(prev, false)
	return in.set(cur, true)
}

func (in *Input) hat(joy sdl.JoystickID, v uint8) []Action {
	prev := in.hatDir[joy]
	in.hatDir[joy] = v
	var out []Action
	for _, d := range []struct {
		bit uint8
		a   Action
	}{{sdl.HAT_UP, Up}, {sdl.HAT_DOWN, Down}, {sdl.HAT_LEFT, Left}, {sdl.HAT_RIGHT, Right}} {
		was, is := prev&d.bit != 0, v&d.bit != 0
		if was != is {
			out = append(out, in.set(d.a, is)...)
		}
	}
	return out
}

func (in *Input) controllerButton(b uint8) Action {
	// Handheld mapping DBs (muOS, ROCKNIX, PortMaster) name buttons after
	// their printed labels, so SDL "a" is the button marked A.
	switch b {
	case sdl.CONTROLLER_BUTTON_A:
		if in.swapAB {
			return B
		}
		return A
	case sdl.CONTROLLER_BUTTON_B:
		if in.swapAB {
			return A
		}
		return B
	case sdl.CONTROLLER_BUTTON_X:
		return X
	case sdl.CONTROLLER_BUTTON_Y:
		return Y
	case sdl.CONTROLLER_BUTTON_LEFTSHOULDER:
		return L1
	case sdl.CONTROLLER_BUTTON_RIGHTSHOULDER:
		return R1
	case sdl.CONTROLLER_BUTTON_BACK:
		return Select
	case sdl.CONTROLLER_BUTTON_START:
		return Start
	case sdl.CONTROLLER_BUTTON_GUIDE:
		return Menu
	case sdl.CONTROLLER_BUTTON_DPAD_UP:
		return Up
	case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
		return Down
	case sdl.CONTROLLER_BUTTON_DPAD_LEFT:
		return Left
	case sdl.CONTROLLER_BUTTON_DPAD_RIGHT:
		return Right
	}
	return None
}

func keyAction(k sdl.Keycode) Action {
	switch k {
	case sdl.K_UP:
		return Up
	case sdl.K_DOWN:
		return Down
	case sdl.K_LEFT:
		return Left
	case sdl.K_RIGHT:
		return Right
	case sdl.K_RETURN, sdl.K_KP_ENTER:
		return A
	case sdl.K_ESCAPE, sdl.K_BACKSPACE:
		return B
	case sdl.K_x:
		return X
	case sdl.K_y:
		return Y
	case sdl.K_q, sdl.K_PAGEUP:
		return L1
	case sdl.K_e, sdl.K_PAGEDOWN:
		return R1
	case sdl.K_TAB:
		return Select
	case sdl.K_SPACE:
		return Start
	case sdl.K_F10:
		return Menu
	}
	return None
}

func (in *Input) open(index int) {
	if sdl.IsGameController(index) {
		if c := sdl.GameControllerOpen(index); c != nil {
			in.controllers[c.Joystick().InstanceID()] = c
			log.Printf("input: controller %q: %s", c.Name(), c.Mapping())
		}
		return
	}
	if j := sdl.JoystickOpen(index); j != nil {
		in.joysticks[j.InstanceID()] = j
		log.Printf("input: joystick without mapping %q, using joystick_buttons", j.Name())
	}
}

func (in *Input) close(id sdl.JoystickID) {
	if c, ok := in.controllers[id]; ok {
		c.Close()
		delete(in.controllers, id)
	}
	if j, ok := in.joysticks[id]; ok {
		j.Close()
		delete(in.joysticks, id)
	}
}

// applyMapping installs a mapping built from wizard bindings for the pad
// behind joystick instance id, stores it in cfg and reopens the pads so it
// takes effect right away.
func (in *Input) applyMapping(id sdl.JoystickID, got map[string]string, cfg *config.Config) error {
	var joy *sdl.Joystick
	if c, ok := in.controllers[id]; ok {
		joy = c.Joystick()
	} else if j, ok := in.joysticks[id]; ok {
		joy = j
	}
	if joy == nil {
		return errors.New("gamepad disconnected")
	}
	guid := guidString(joy.GUID())
	mapping := buildMapping(guid, joy.Name(), sdl.GameControllerMappingForGUID(joy.GUID()), got)
	if sdl.GameControllerAddMapping(mapping) < 0 {
		return errors.New(sdl.GetError().Error())
	}
	if cfg.ControllerMappings == nil {
		cfg.ControllerMappings = map[string]string{}
	}
	cfg.ControllerMappings[guid] = mapping
	log.Printf("input: new mapping %s", mapping)

	in.closeAll()
	n := sdl.NumJoysticks()
	for i := 0; i < n; i++ {
		in.open(i)
	}
	return nil
}

// guidString formats a joystick GUID as SDL does. go-sdl2's
// JoystickGetGUIDString passes a too-small buffer and truncates it.
func guidString(g sdl.JoystickGUID) string {
	b := *(*[16]byte)(unsafe.Pointer(&g))
	return hex.EncodeToString(b[:])
}

// addMappings installs saved per-pad mappings; call before pads are opened.
func addMappings(cfg *config.Config) {
	for guid, m := range cfg.ControllerMappings {
		if len(guid) != 32 {
			log.Printf("input: ignoring mapping with malformed GUID %q; run button setup again", guid)
			continue
		}
		if sdl.GameControllerAddMapping(m) < 0 {
			log.Printf("input: bad saved mapping for %s: %v", guid, sdl.GetError())
		}
	}
}

func (in *Input) closeAll() {
	for id := range in.controllers {
		in.close(id)
	}
	for id := range in.joysticks {
		in.close(id)
	}
}

var actionLabels = [...]string{"none", "up", "down", "left", "right", "a", "b", "x", "y", "l1", "r1", "select", "start", "menu"}

func (a Action) String() string {
	if int(a) < len(actionLabels) {
		return actionLabels[a]
	}
	return strconv.Itoa(int(a))
}
