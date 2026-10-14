package database

import (
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

func TestNormalizeDSNToUTC(t *testing.T) {
	input := "keyhub:keyhub@tcp(127.0.0.1:3306)/keyhub?parseTime=true&loc=Local&charset=utf8mb4,utf8"

	normalized, err := normalizeDSNToUTC(input)
	if err != nil {
		t.Fatalf("normalize DSN: %v", err)
	}

	cfg, err := mysql.ParseDSN(normalized)
	if err != nil {
		t.Fatalf("parse normalized DSN: %v", err)
	}
	if !cfg.ParseTime {
		t.Fatal("ParseTime = false, want true")
	}
	if cfg.Loc != time.UTC {
		t.Fatalf("Loc = %v, want UTC", cfg.Loc)
	}
	if got := cfg.Params["time_zone"]; got != "'+00:00'" {
		t.Fatalf("time_zone = %q, want %q", got, "'+00:00'")
	}
}

func TestNormalizeDSNToUTCAddsRequiredTimeSettings(t *testing.T) {
	normalized, err := normalizeDSNToUTC("keyhub:keyhub@tcp(127.0.0.1:3306)/keyhub")
	if err != nil {
		t.Fatalf("normalize DSN: %v", err)
	}

	cfg, err := mysql.ParseDSN(normalized)
	if err != nil {
		t.Fatalf("parse normalized DSN: %v", err)
	}
	if !cfg.ParseTime || cfg.Loc != time.UTC || cfg.Params["time_zone"] != "'+00:00'" {
		t.Fatalf("normalized time settings are incomplete: ParseTime=%v Loc=%v time_zone=%q",
			cfg.ParseTime, cfg.Loc, cfg.Params["time_zone"])
	}
}

func TestNormalizeDSNToUTCRejectsInvalidDSN(t *testing.T) {
	if _, err := normalizeDSNToUTC("not a mysql dsn"); err == nil {
		t.Fatal("normalizeDSNToUTC() error = nil, want invalid DSN error")
	}
}
