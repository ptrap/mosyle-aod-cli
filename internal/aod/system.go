package aod

import (
	"context"
	"encoding/xml"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type System struct{ User, Home string }

func LocalSystem(ctx context.Context) (System, error) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return System{}, ErrPlatform
	}
	if os.Getuid() == 0 || os.Geteuid() != os.Getuid() {
		return System{}, ErrUser
	}
	u, err := user.LookupId(stringUID())
	if err != nil {
		return System{}, ErrUser
	}
	s := System{User: u.Username, Home: u.HomeDir}
	console, err := os.Stat("/dev/console")
	if err != nil {
		return System{}, errors.New("could not inspect the desktop session")
	}
	stat, ok := console.Sys().(*syscall.Stat_t)
	if !ok || strconv.FormatUint(uint64(stat.Uid), 10) != u.Uid {
		return System{}, ErrUser
	}
	return s, nil
}

func stringUID() string { return strconv.Itoa(os.Getuid()) }

func command(ctx context.Context, path string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, path, args...).Output()
}

func (s System) Admin(ctx context.Context) (bool, error) {
	_, err := command(ctx, "/usr/sbin/dseditgroup", "-o", "checkmember", "-m", s.User, "admin")
	if err == nil {
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 67 {
		return false, nil
	}
	return false, errors.New("could not read local admin-group membership")
}

func (s System) Device(ctx context.Context) (string, string, error) {
	data, err := command(ctx, "/usr/sbin/ioreg", "-rd1", "-c", "IOPlatformExpertDevice", "-a")
	if err != nil {
		return "", "", errors.New("could not read local device identifier")
	}
	device := plistValue(data, "IOPlatformUUID")
	build, err := command(ctx, "/usr/libexec/PlistBuddy", "-c", "Print :CFBundleVersion", "/Applications/Self-Service.app/Contents/Info.plist")
	if err != nil || device == "" {
		return "", "", errors.New("Mosyle Self-Service app or device identifier unavailable")
	}
	return device, strings.TrimSpace(string(build)), nil
}

func plistValue(data []byte, key string) string {
	d := xml.NewDecoder(strings.NewReader(string(data)))
	found := false
	for {
		t, e := d.Token()
		if e != nil {
			return ""
		}
		s, ok := t.(xml.StartElement)
		if !ok {
			continue
		}
		if s.Name.Local == "key" {
			var v string
			if d.DecodeElement(&v, &s) != nil {
				return ""
			}
			found = v == key
		} else if found && s.Name.Local == "string" {
			var v string
			if d.DecodeElement(&v, &s) != nil {
				return ""
			}
			return v
		}
	}
}

func (s System) Client() (*Client, error) {
	path := filepath.Join(s.Home, "Library/HTTPStorages/com.mosyle.macos.business.binarycookies")
	file, err := os.Open(path)
	if err != nil {
		return nil, ErrSession
	}
	defer file.Close()
	data, err := readLimited(file, 4<<20)
	if err != nil {
		return nil, ErrSession
	}
	cookies, err := ParseCookies(data, time.Now())
	if err != nil || len(cookies) == 0 {
		return nil, ErrSession
	}
	return NewClient(cookies), nil
}
