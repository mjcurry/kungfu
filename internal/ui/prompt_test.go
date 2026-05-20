package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestSelectAcceptsDefault(t *testing.T) {
	in := strings.NewReader("\n")
	out := &bytes.Buffer{}
	p := NewPrompterFor(in, out)
	got, err := p.Select("Pick one", []SelectOption{
		{"basic", "minimal"},
		{"document", "for docs"},
	}, "document")
	if err != nil {
		t.Fatal(err)
	}
	if got != "document" {
		t.Errorf("got %q, want document (default)", got)
	}
}

func TestSelectByNumber(t *testing.T) {
	p := NewPrompterFor(strings.NewReader("2\n"), &bytes.Buffer{})
	got, err := p.Select("Pick one", []SelectOption{
		{"basic", ""},
		{"document", ""},
	}, "basic")
	if err != nil {
		t.Fatal(err)
	}
	if got != "document" {
		t.Errorf("got %q, want document", got)
	}
}

func TestSelectByName(t *testing.T) {
	p := NewPrompterFor(strings.NewReader("Data\n"), &bytes.Buffer{})
	got, err := p.Select("Pick one", []SelectOption{
		{"basic", ""}, {"data", ""},
	}, "basic")
	if err != nil {
		t.Fatal(err)
	}
	if got != "data" {
		t.Errorf("got %q, want data", got)
	}
}

func TestInputAcceptsDefault(t *testing.T) {
	p := NewPrompterFor(strings.NewReader("\n"), &bytes.Buffer{})
	got, err := p.Input("Name", "hint", "fallback", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "fallback" {
		t.Errorf("got %q, want fallback", got)
	}
}

func TestInputValidatesAndReprompts(t *testing.T) {
	// First line "bad" fails validation, second "good" passes.
	p := NewPrompterFor(strings.NewReader("bad\ngood\n"), &bytes.Buffer{})
	validate := func(s string) error {
		if s == "good" {
			return nil
		}
		return errors.New("must be good")
	}
	got, err := p.Input("Name", "", "", validate)
	if err != nil {
		t.Fatal(err)
	}
	if got != "good" {
		t.Errorf("got %q, want good", got)
	}
}

func TestConfirmDefaultYes(t *testing.T) {
	p := NewPrompterFor(strings.NewReader("\n"), &bytes.Buffer{})
	ok, err := p.Confirm("Proceed?", true)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("default yes should return true on empty input")
	}
}

func TestConfirmDefaultNo(t *testing.T) {
	p := NewPrompterFor(strings.NewReader("\n"), &bytes.Buffer{})
	ok, err := p.Confirm("Proceed?", false)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("default no should return false on empty input")
	}
}

func TestConfirmExplicit(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{{"y\n", true}, {"yes\n", true}, {"N\n", false}, {"no\n", false}}
	for _, tc := range cases {
		p := NewPrompterFor(strings.NewReader(tc.input), &bytes.Buffer{})
		got, err := p.Confirm("Proceed?", true)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("input %q: got %v, want %v", tc.input, got, tc.want)
		}
	}
}
