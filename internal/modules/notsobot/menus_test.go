package notsobot

import (
	"reflect"
	"slices"
	"testing"

	"mayuri2000/internal/cmd"
	"mayuri2000/internal/imagefind"
	"mayuri2000/internal/imaging"
)

func TestMenus(t *testing.T) {
	m := New(imagefind.New(""), &imaging.Magick{}, &imaging.FFmpeg{})
	cmds := m.Commands()
	runs := map[string]uintptr{}
	for _, c := range cmds {
		runs[c.Name] = reflect.ValueOf(c.Run).Pointer()
	}

	var message, user []string
	for _, menu := range m.Menus(cmds) {
		if menu.Kind == cmd.MenuMessage {
			message = append(message, menu.Name)
		} else {
			user = append(user, menu.Name)
		}
		if menu.Name == "GMagik" && reflect.ValueOf(menu.Run).Pointer() != runs["gmagik"] {
			t.Error("GMagik menu doesn't run gmagik")
		}
	}
	if want := []string{"Magik", "JPEG", "Glitch", "GMagik", "Invert"}; !slices.Equal(message, want) {
		t.Errorf("message menus = %v, want %v", message, want)
	}
	if want := []string{"Triggered", "Pet"}; !slices.Equal(user, want) {
		t.Errorf("user menus = %v, want %v", user, want)
	}
	// Vaporwave left the Apps menu but stays a command.
	if _, ok := runs["vw"]; !ok {
		t.Error("vw command missing")
	}
}
