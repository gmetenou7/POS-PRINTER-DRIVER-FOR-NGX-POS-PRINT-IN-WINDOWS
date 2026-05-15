//go:build !windows

package tlsmgr

import "errors"

var errNotWindows = errors.New("trust-store helpers are Windows-only")

func TrustCA(caCertPath string) error        { return errNotWindows }
func UntrustCA(commonName string) error      { return errNotWindows }
