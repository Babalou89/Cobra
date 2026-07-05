package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func passing() Proposal {
	return Proposal{
		Name:        "word-count",
		Description: "count words on stdin",
		Usage:       "echo text | run.sh",
		RunSh:       "#!/bin/bash\nwc -w\n",
		TestSh:      "#!/bin/bash\nset -e\nout=$(echo 'one two three' | bash run.sh)\ntest \"$out\" -eq 3\n",
	}
}

func TestGateAcceptsProvenSkill(t *testing.T) {
	dir, err := Stage(t.TempDir(), passing(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := Gate(dir, 10*time.Second); err != nil {
		t.Fatalf("passing skill rejected: %v", err)
	}
}

func TestGateRejectsFailingSkill(t *testing.T) {
	p := passing()
	p.Name = "broken"
	p.TestSh = "#!/bin/bash\nexit 1\n"
	dir, err := Stage(t.TempDir(), p, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := Gate(dir, 10*time.Second); err == nil {
		t.Fatal("failing test must reject the skill")
	}
}

func TestGateRejectsHangingSkill(t *testing.T) {
	p := passing()
	p.Name = "hangs"
	p.TestSh = "#!/bin/bash\nsleep 60\n"
	dir, err := Stage(t.TempDir(), p, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := Gate(dir, 1*time.Second); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("hanging test must time out, got: %v", err)
	}
}

func TestValidateRejectsGarbage(t *testing.T) {
	cases := []Proposal{
		{Name: "Bad Name", Description: "x", RunSh: "y", TestSh: "z"},
		{Name: "no-test", Description: "x", RunSh: "y", TestSh: "  "},
		{Name: "no-desc", Description: " ", RunSh: "y", TestSh: "z"},
		{Name: "evil", Description: "x", RunSh: "sudo rm -rf /", TestSh: "z"},
	}
	for _, p := range cases {
		if err := Validate(p); err == nil {
			t.Errorf("proposal %+v must be rejected", p.Name)
		}
	}
}

func TestInstallRefusesOverwrite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	staging := t.TempDir()

	dir, err := Stage(staging, passing(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(root, dir); err != nil {
		t.Fatal(err)
	}
	dir2, err := Stage(staging, passing(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(root, dir2); err == nil {
		t.Fatal("second install of the same name must be refused")
	}
}

func TestListAndDescribe(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	staging := t.TempDir()
	dir, err := Stage(staging, passing(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(root, dir); err != nil {
		t.Fatal(err)
	}
	list, err := List(root)
	if err != nil || len(list) != 1 || list[0].Name != "word-count" {
		t.Fatalf("list = %+v, err = %v", list, err)
	}
	if !strings.Contains(Describe(list), "word-count") {
		t.Fatal("describe must mention the skill")
	}
	if _, err := os.Stat(filepath.Join(list[0].Dir, "run.sh")); err != nil {
		t.Fatal("installed skill missing run.sh")
	}
}
