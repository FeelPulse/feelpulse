package tts

import (
	"os/exec"
	"testing"
)

func TestDetectCommand(t *testing.T) {
	// DetectCommand should return an available TTS command or empty string
	cmd := DetectCommand()
	
	// If we found a command, verify it exists
	if cmd != "" {
		_, err := exec.LookPath(cmd)
		if err != nil {
			t.Errorf("DetectCommand returned %q but it's not in PATH", cmd)
		}
	}
	// It's OK for cmd to be empty if no TTS is available
}

func TestSpeaker_BuildCommand(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		text     string
		wantCmd  string
		wantArgs []string
	}{
		{
			name:     "espeak basic",
			command:  "espeak",
			text:     "Hello world",
			wantCmd:  "espeak",
			wantArgs: []string{"Hello world"},
		},
		{
			name:     "say basic",
			command:  "say",
			text:     "Hello world",
			wantCmd:  "say",
			wantArgs: []string{"Hello world"},
		},
		{
			name:     "festival basic",
			command:  "festival",
			text:     "Hello world",
			wantCmd:  "festival",
			wantArgs: []string{"--tts"},
		},
		{
			name:     "espeak-ng basic",
			command:  "espeak-ng",
			text:     "Hello world",
			wantCmd:  "espeak-ng",
			wantArgs: []string{"Hello world"},
		},
		{
			name:     "custom command",
			command:  "my-tts",
			text:     "Hello world",
			wantCmd:  "my-tts",
			wantArgs: []string{"Hello world"},
		},
		{
			name:     "text with quotes",
			command:  "espeak",
			text:     `He said "hello"`,
			wantCmd:  "espeak",
			wantArgs: []string{`He said "hello"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Speaker{Command: tt.command}
			gotCmd, gotArgs, needsStdin := s.BuildCommand(tt.text)

			if gotCmd != tt.wantCmd {
				t.Errorf("BuildCommand() cmd = %q, want %q", gotCmd, tt.wantCmd)
			}

			// For festival, it uses stdin
			if tt.command == "festival" {
				if !needsStdin {
					t.Error("festival should need stdin")
				}
			} else {
				if needsStdin {
					t.Error("non-festival command should not need stdin")
				}
				if len(gotArgs) != len(tt.wantArgs) {
					t.Errorf("BuildCommand() args len = %d, want %d", len(gotArgs), len(tt.wantArgs))
				} else {
					for i, a := range gotArgs {
						if a != tt.wantArgs[i] {
							t.Errorf("BuildCommand() args[%d] = %q, want %q", i, a, tt.wantArgs[i])
						}
					}
				}
			}
		})
	}
}

func TestSpeaker_Speak_NoCommand(t *testing.T) {
	s := &Speaker{Command: ""}
	err := s.Speak("Hello")
	if err != ErrNoTTSCommand {
		t.Errorf("Speak() with no command should return ErrNoTTSCommand, got %v", err)
	}
}

func TestSpeaker_Speak_EmptyText(t *testing.T) {
	s := &Speaker{Command: "espeak"}
	err := s.Speak("")
	// Empty text should be a no-op, not an error
	if err != nil {
		t.Errorf("Speak() with empty text should not error, got %v", err)
	}
}

func TestSpeaker_Available(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{
			name:    "empty command",
			command: "",
			want:    false,
		},
		{
			name:    "nonexistent command",
			command: "definitely-not-a-real-tts-command-12345",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Speaker{Command: tt.command}
			got := s.Available()
			if got != tt.want {
				t.Errorf("Available() = %v, want %v", got, tt.want)
			}
		})
	}

	// Test with a command that exists (if any TTS is available)
	detected := DetectCommand()
	if detected != "" {
		s := &Speaker{Command: detected}
		if !s.Available() {
			t.Errorf("Available() = false for detected command %q", detected)
		}
	}
}

func TestNew(t *testing.T) {
	// Test with empty command - should auto-detect
	s := New("")
	if s == nil {
		t.Error("New() returned nil")
	}

	// Test with explicit command
	s = New("custom-tts")
	if s.Command != "custom-tts" {
		t.Errorf("New(custom-tts) command = %q, want %q", s.Command, "custom-tts")
	}
}

func TestSpeaker_SanitizeText(t *testing.T) {
	s := &Speaker{Command: "espeak"}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "normal text",
			input: "Hello world",
			want:  "Hello world",
		},
		{
			name:  "markdown bold",
			input: "This is **bold** text",
			want:  "This is bold text",
		},
		{
			name:  "markdown italic",
			input: "This is *italic* text",
			want:  "This is italic text",
		},
		{
			name:  "code blocks",
			input: "Run `go test` to test",
			want:  "Run go test to test",
		},
		{
			name:  "links",
			input: "Check [this link](https://example.com)",
			want:  "Check this link",
		},
		{
			name:  "emoji",
			input: "🔄 Starting fresh! 🎉",
			want:  "Starting fresh!",
		},
		{
			name:  "multiple spaces",
			input: "Hello    world",
			want:  "Hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.SanitizeText(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
