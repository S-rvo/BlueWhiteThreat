package scraper_tests

import (
	"os"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/utils"
)

func TestGetEnvOrDefault_ReturnsEnvValue(t *testing.T) {
    os.Setenv("FOO_BAR", "hello")
    defer os.Unsetenv("FOO_BAR")

    val := utils.GetEnvOrDefault("FOO_BAR", "default")
    if val != "hello" {
        t.Errorf("attendu 'hello', obtenu '%s'", val)
    }
}

func TestGetEnvOrDefault_ReturnsDefault(t *testing.T) {
    os.Unsetenv("FOO_BAR")  // au cas où il existe déjà

    val := utils.GetEnvOrDefault("FOO_BAR", "default")
    if val != "default" {
        t.Errorf("attendu 'default', obtenu '%s'", val)
    }
}
