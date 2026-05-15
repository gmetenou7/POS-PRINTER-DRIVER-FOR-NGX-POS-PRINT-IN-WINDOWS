//go:build !windows

package service

import (
	"context"
	"errors"
)

var errNotWindows = errors.New("le mode service n'est disponible que sur Windows")

func RunService(name string, run func(ctx context.Context) error) error { return errNotWindows }
func Install(name, desc string) error                                   { return errNotWindows }
func Uninstall(name string) error                                       { return errNotWindows }
func Start(name string) error                                           { return errNotWindows }
func Stop(name string) error                                            { return errNotWindows }
func Status(name string) (string, error)                                { return "", errNotWindows }
