// Packer-provisioner-tunnel is a packer provisioner plugin.
//
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/mitchellh/packer/common"
	"github.com/mitchellh/packer/helper/config"
	"github.com/mitchellh/packer/packer"
	"github.com/mitchellh/packer/packer/plugin"
	"github.com/mitchellh/packer/template/interpolate"
)

type tunnel struct {
	common.PackerConfig `mapstructure:",squash"`

	Exec string   `mapstructure:"exec"`
	Args []string `mapstructure:"args"`

	server *sshServer
}

func (t *tunnel) Prepare(raw ...interface{}) error {
	var errs *packer.MultiError

	err := config.Decode(t, nil, raw...)
	if err != nil {
		return err
	}

	if t.Exec == "" {
		errs = packer.MultiErrorAppend(errs, fmt.Errorf("missing tunnel provisioner parameter exec"))
	}

	t.Exec, err = interpolate.Render(t.Exec, nil)
	if err != nil {
		errs = packer.MultiErrorAppend(errs, fmt.Errorf("error processing exec template: %s", err))
	}

	for i, arg := range t.Args {
		t.Args[i], err = interpolate.Render(arg, nil)
		if err != nil {
			errs = packer.MultiErrorAppend(errs, fmt.Errorf("error processing arg %d (%q): %s", i, arg, err))
		}
	}

	if errs != nil && len(errs.Errors) > 0 {
		return errs
	}

	var texec string
	texec, err = exec.LookPath(t.Exec)
	if err != nil {
		return fmt.Errorf("executable %q not found: %v", t.Exec, err)
	}
	t.Exec = texec

	t.server, err = newSSHServer()
	if err != nil {
		return fmt.Errorf("could not initialize ssh server: %v", err)
	}
	return nil
}

func (t *tunnel) Provision(ui packer.Ui, comm packer.Communicator) error {
	ui.Say("Starting tunnel")
	t.server.comm = comm
	errc := make(chan error, 1)
	go func() {
		errc <- t.server.serveOne()
	}()

	cmd := exec.Command(t.Exec, t.Args...)
	cmd.Env = append(os.Environ(),
		"PACKER_TUNNEL_USERNAME="+t.server.username,
		"PACKER_TUNNEL_PASSWORD="+t.server.password,
		"PACKER_TUNNEL_PORT="+strconv.Itoa(t.server.port),
	)
	log.Println("Command", cmd.Args, "env", cmd.Env)

	ui.Say("Running command " + strings.Join(cmd.Args, " "))
	out, err := cmd.CombinedOutput()
	ui.Say(string(out))
	if err != nil {
		ui.Say(fmt.Sprintf("Error running command %s", err))
		return err
	}

	return <-errc
}

func (t *tunnel) Cancel() {
	log.Println("Cancelled")
	os.Exit(0)
}

func main() {
	server, err := plugin.Server()
	if err != nil {
		panic(err)
	}
	server.RegisterProvisioner(new(tunnel))
	server.Serve()
}
