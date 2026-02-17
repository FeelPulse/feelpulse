// Package tts provides text-to-speech functionality
package tts

import (
	"errors"
	"os/exec"
	"regexp"
	"strings"
)

// ErrNoTTSCommand is returned when no TTS command is configured or available
var ErrNoTTSCommand = errors.New("no TTS command available")

// Speaker handles text-to-speech output
type Speaker struct {
	Command string // The TTS command to use (espeak, say, festival, etc.)
}

// New creates a new Speaker with the given command.
// If command is empty, it will auto-detect an available TTS command.
func New(command string) *Speaker {
	if command == "" {
		command = DetectCommand()
	}
	return &Speaker{Command: command}
}

// DetectCommand attempts to find an available TTS command on the system.
// Returns empty string if none found.
func DetectCommand() string {
	// List of TTS commands to try, in order of preference
	commands := []string{
		"say",       // macOS
		"espeak-ng", // Modern espeak
		"espeak",    // Linux
		"festival",  // Festival TTS
	}

	for _, cmd := range commands {
		if _, err := exec.LookPath(cmd); err == nil {
			return cmd
		}
	}
	return ""
}

// Available returns true if the TTS command is available
func (s *Speaker) Available() bool {
	if s.Command == "" {
		return false
	}
	_, err := exec.LookPath(s.Command)
	return err == nil
}

// BuildCommand builds the command and arguments for the given text.
// Returns the command, arguments, and whether stdin is needed.
func (s *Speaker) BuildCommand(text string) (cmd string, args []string, needsStdin bool) {
	switch s.Command {
	case "festival":
		// Festival reads from stdin with --tts flag
		return s.Command, []string{"--tts"}, true
	case "espeak", "espeak-ng":
		// espeak takes text as argument
		return s.Command, []string{text}, false
	case "say":
		// macOS say takes text as argument
		return s.Command, []string{text}, false
	default:
		// Generic fallback: pass text as argument
		return s.Command, []string{text}, false
	}
}

// Speak synthesizes the given text to speech
func (s *Speaker) Speak(text string) error {
	// Empty text is a no-op
	if strings.TrimSpace(text) == "" {
		return nil
	}

	// No command configured
	if s.Command == "" {
		return ErrNoTTSCommand
	}

	// Sanitize text for speech
	text = s.SanitizeText(text)
	if text == "" {
		return nil
	}

	// Build command
	cmdName, args, needsStdin := s.BuildCommand(text)

	cmd := exec.Command(cmdName, args...)

	if needsStdin {
		// For commands like festival that read from stdin
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}

		if err := cmd.Start(); err != nil {
			return err
		}

		_, _ = stdin.Write([]byte(text))
		stdin.Close()

		return cmd.Wait()
	}

	// For commands that take text as argument
	return cmd.Run()
}

// SanitizeText removes markdown, emoji, and other non-speakable content
func (s *Speaker) SanitizeText(text string) string {
	// Remove emoji (basic unicode emoji ranges)
	emojiRe := regexp.MustCompile(`[\x{1F300}-\x{1F9FF}\x{2600}-\x{26FF}\x{2700}-\x{27BF}]`)
	text = emojiRe.ReplaceAllString(text, "")

	// Remove markdown links: [text](url) -> text
	linkRe := regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	text = linkRe.ReplaceAllString(text, "$1")

	// Remove markdown bold: **text** or __text__ -> text
	boldRe := regexp.MustCompile(`\*\*([^*]+)\*\*|__([^_]+)__`)
	text = boldRe.ReplaceAllString(text, "$1$2")

	// Remove markdown italic: *text* or _text_ -> text
	italicRe := regexp.MustCompile(`\*([^*]+)\*|_([^_]+)_`)
	text = italicRe.ReplaceAllString(text, "$1$2")

	// Remove inline code: `code` -> code
	codeRe := regexp.MustCompile("`([^`]+)`")
	text = codeRe.ReplaceAllString(text, "$1")

	// Remove code blocks
	codeBlockRe := regexp.MustCompile("```[\\s\\S]*?```")
	text = codeBlockRe.ReplaceAllString(text, "")

	// Collapse multiple spaces
	spaceRe := regexp.MustCompile(`\s+`)
	text = spaceRe.ReplaceAllString(text, " ")

	return strings.TrimSpace(text)
}
