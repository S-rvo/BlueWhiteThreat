package utils

import (
	"os"
	"testing"
)

func TestGetEnvOrDefault_ReturnsEnvValue(t *testing.T) {
    os.Setenv("FOO_BAR", "hello")
    defer os.Unsetenv("FOO_BAR")

    val := GetEnvOrDefault("FOO_BAR", "default")
    if val != "hello" {
        t.Errorf("attendu 'hello', obtenu '%s'", val)
    }
}

func TestGetEnvOrDefault_ReturnsDefault(t *testing.T) {
    os.Unsetenv("FOO_BAR")  // au cas où il existe déjà

    val := GetEnvOrDefault("FOO_BAR", "default")
    if val != "default" {
        t.Errorf("attendu 'default', obtenu '%s'", val)
    }
}
