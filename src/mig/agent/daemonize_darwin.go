/* Mozilla InvestiGator Agent

Version: MPL 1.1/GPL 2.0/LGPL 2.1

The contents of this file are subject to the Mozilla Public License Version
1.1 (the "License"); you may not use this file except in compliance with
the License. You may obtain a copy of the License at
http://www.mozilla.org/MPL/

Software distributed under the License is distributed on an "AS IS" basis,
WITHOUT WARRANTY OF ANY KIND, either express or implied. See the License
for the specific language governing rights and limitations under the
License.

The Initial Developer of the Original Code is
Mozilla Corporation
Portions created by the Initial Developer are Copyright (C) 2014
the Initial Developer. All Rights Reserved.

Contributor(s):
Julien Vehent jvehent@mozilla.com [:ulfr]

Alternatively, the contents of this file may be used under the terms of
either the GNU General Public License Version 2 or later (the "GPL"), or
the GNU Lesser General Public License Version 2.1 or later (the "LGPL"),
in which case the provisions of the GPL or the LGPL are applicable instead
of those above. If you wish to allow use of your version of this file only
under the terms of either the GPL or the LGPL, and not to allow others to
use your version of this file under the terms of the MPL, indicate your
decision by deleting the provisions above and replace them with the notice
and other provisions required by the GPL or the LGPL. If you do not delete
the provisions above, a recipient may use your version of this file under
the terms of any one of the MPL, the GPL or the LGPL.
*/

package main

import (
	"fmt"
	"mig"
	"os"
	"os/exec"
	"time"
)

// On MacOS, launchd takes care of keeping processes alive. The daemonization
// procedure consist of installing and starting the service, then exiting.
// Launchd will take care of daemonizing the agent
func daemonize(orig_ctx Context) (ctx Context, err error) {
	ctx = orig_ctx
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("daemonize() -> %v", e)
		}
		ctx.Channels.Log <- mig.Log{Desc: "leaving daemonize()"}.Debug()
	}()

	if os.Getppid() == 1 {
		// if this agent has been launched as part of an upgrade, its parent will be
		// detached to init, but no service would be launched, so we launch one
		if upgrading && MUSTINSTALLSERVICE {
			ctx.Channels.Log <- mig.Log{Desc: "Agent is an upgrade. Deploying service."}.Debug()
			time.Sleep(3 * time.Second)
			ctx, err = serviceDeploy(ctx)
			if err != nil {
				panic(err)
			}
			// mig-agent service has been launched, exit this process
			ctx.Channels.Log <- mig.Log{Desc: "Service deployed. Exit."}.Debug()
			os.Exit(0)
		}
		// if controlled by launchd, we tell the agent
		// to not respawn itself. launchd will do it
		ctx.Channels.Log <- mig.Log{Desc: "Running as a service."}.Debug()
		ctx.Agent.Respawn = false
	} else {
		// install the service, start it, and exit
		if MUSTINSTALLSERVICE {
			ctx, err = serviceDeploy(ctx)
			if err != nil {
				panic(err)
			}
			ctx.Channels.Log <- mig.Log{Desc: "Service deployed. Exit."}.Debug()
		} else {
			// we are not in foreground mode, and we don't want a service installation
			// so just fork in foreground mode, and exit the current process
			cmd := exec.Command(ctx.Agent.BinPath, "-f")
			err = cmd.Start()
			if err != nil {
				ctx.Channels.Log <- mig.Log{Desc: fmt.Sprintf("Failed to spawn new agent from '%s': '%v'", ctx.Agent.BinPath, err)}.Err()
				return ctx, err
			}
			ctx.Channels.Log <- mig.Log{Desc: "Started new foreground agent. Exit."}.Debug()
		}
		os.Exit(0)
	}
	return
}
