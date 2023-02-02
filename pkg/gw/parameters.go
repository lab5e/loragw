package gw

import "github.com/lab5e/mofunk/pkg/moclient"

// Parameters holds the main command line parameters for the gateway interface
type Parameters struct {
	CertFile  string              `kong:"help='Client Certificate',required,file"`
	Chain     string              `kong:"help='Certificate chain',required,file"`
	KeyFile   string              `kong:"help='Client key file',required,file"`
	StateFile string              `kong:"help='State file for gateway',default=''"`
	Cluster   moclient.Parameters `kong:"embed,prefix='cluster-'"` // TODO (stalehd): Temporary dependency
}
