//go:build windows

// Package service wraps the golang.org/x/sys/windows/svc helpers so that
// the agent can run as a console process, a Windows service, or be
// installed/removed from the SCM with a single binary.
package service

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type runFunc func(ctx context.Context) error

// RunService is called by the agent's "-cmd run" mode (when SCM launches us).
// It dispatches into the Windows Service Control loop, translating SCM
// signals into context cancellation for the inner runFunc.
func RunService(name string, run runFunc) error {
	return svc.Run(name, &handler{run: run})
}

type handler struct {
	run runFunc
}

func (h *handler) Execute(args []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	const accepts = svc.AcceptStop | svc.AcceptShutdown

	s <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- h.run(ctx) }()

	s <- svc.Status{State: svc.Running, Accepts: accepts}

	for {
		select {
		case req := <-r:
			switch req.Cmd {
			case svc.Interrogate:
				s <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				s <- svc.Status{State: svc.StopPending}
				cancel()
				<-errCh
				s <- svc.Status{State: svc.Stopped}
				return false, 0
			}
		case err := <-errCh:
			s <- svc.Status{State: svc.StopPending}
			if err != nil && err != context.Canceled {
				s <- svc.Status{State: svc.Stopped, Win32ExitCode: 1}
				return false, 1
			}
			s <- svc.Status{State: svc.Stopped}
			return false, 0
		}
	}
}

// Install registers the agent binary as a Windows service.
func Install(name, desc string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect SCM: %w", err)
	}
	defer m.Disconnect()

	if s, err := m.OpenService(name); err == nil {
		s.Close()
		return fmt.Errorf("service %q already installed", name)
	}

	s, err := m.CreateService(name, exe, mgr.Config{
		DisplayName: "Print Bridge",
		Description: desc,
		StartType:   mgr.StartAutomatic,
	}, "-cmd", "run")
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer s.Close()
	return nil
}

func Uninstall(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("service %q not found", name)
	}
	defer s.Close()
	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	return nil
}

func Start(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.Start()
}

func Stop(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return err
	}
	defer s.Close()
	_, err = s.Control(svc.Stop)
	return err
}

func Status(name string) (string, error) {
	m, err := mgr.Connect()
	if err != nil {
		return "", err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return "non installé", nil
	}
	defer s.Close()
	st, err := s.Query()
	if err != nil {
		return "", err
	}
	switch st.State {
	case svc.Stopped:
		return "arrêté", nil
	case svc.StartPending:
		return "démarrage…", nil
	case svc.StopPending:
		return "arrêt…", nil
	case svc.Running:
		return "en cours", nil
	case svc.ContinuePending:
		return "reprise…", nil
	case svc.PausePending:
		return "pause…", nil
	case svc.Paused:
		return "en pause", nil
	}
	return fmt.Sprintf("état inconnu (%d)", st.State), nil
}
