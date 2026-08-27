package service

import "strings"

// systemdTemplate is verbatim per AI.md PART 25 "Service Templates" >
// systemd (Linux).
const systemdTemplate = `[Unit]
Description={project_name} service
Documentation=https://{project_org}.github.io/{project_name}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/{project_name}
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

# Security hardening (binary drops privileges after port binding)
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/etc/{internal_org}/{internal_name}
ReadWritePaths=/var/lib/{internal_org}/{internal_name}
ReadWritePaths=/var/cache/{internal_org}/{internal_name}
ReadWritePaths=/var/log/{internal_org}/{internal_name}

[Install]
WantedBy=multi-user.target
`

// openrcTemplate is verbatim per AI.md PART 25 "Service Templates" >
// OpenRC (Alpine, Gentoo, Devuan). Built by concatenation because the
// comment line embeds literal backticks, which a backtick-delimited raw
// string cannot contain.
const openrcTemplate = `#!/sbin/openrc-run
# Service identity comes from {internal_name} so config_dir/data_dir paths stay
# stable across binary renames (see PART 0 → "Why ` + "`" + `{internal_name}` + "`" + ` exists separately from ` + "`" + `{project_name}` + "`" + `").

name="{internal_name}"
description="{app_name}"
# actual binary (may differ from {internal_name} after rename)
command="/usr/local/bin/{project_name}"
command_args=""
command_user="{internal_name}:{internal_name}"
pidfile="/var/run/{internal_org}/{internal_name}.pid"
command_background=true
output_log="/var/log/{internal_org}/{internal_name}/server.log"
error_log="/var/log/{internal_org}/{internal_name}/error.log"

depend() {
    need net
    after firewall
    use dns logger
}

start_pre() {
    checkpath -d -m 0755 -o {internal_name}:{internal_name} /var/run/{internal_org}
    checkpath -d -m 0755 -o {internal_name}:{internal_name} /var/log/{internal_org}/{internal_name}
}
`

// sysvinitTemplate is verbatim per AI.md PART 25 "Service Templates" >
// SysVinit (legacy Linux, init.d).
const sysvinitTemplate = `#!/bin/sh
### BEGIN INIT INFO
# Provides:          {internal_name}
# Required-Start:    $network $remote_fs $syslog
# Required-Stop:     $network $remote_fs $syslog
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: {app_name}
# Description:       {app_name} daemon
### END INIT INFO

NAME={internal_name}
DAEMON=/usr/local/bin/{project_name}
DAEMON_USER={internal_name}
PIDFILE=/var/run/{internal_org}/{internal_name}.pid
LOGFILE=/var/log/{internal_org}/{internal_name}/server.log

case "$1" in
    start)
        echo "Starting $NAME..."
        mkdir -p $(dirname $PIDFILE) $(dirname $LOGFILE)
        chown -R $DAEMON_USER:$DAEMON_USER $(dirname $PIDFILE) $(dirname $LOGFILE)
        start-stop-daemon --start --quiet --background --make-pidfile \
            --pidfile $PIDFILE --chuid $DAEMON_USER --exec $DAEMON \
            --no-close >> $LOGFILE 2>&1
        ;;
    stop)
        echo "Stopping $NAME..."
        start-stop-daemon --stop --quiet --pidfile $PIDFILE --retry 30
        rm -f $PIDFILE
        ;;
    restart)
        $0 stop
        sleep 1
        $0 start
        ;;
    status)
        if [ -f $PIDFILE ] && kill -0 $(cat $PIDFILE) 2>/dev/null; then
            echo "$NAME is running (pid $(cat $PIDFILE))"
            exit 0
        else
            echo "$NAME is stopped"
            exit 3
        fi
        ;;
    *)
        echo "Usage: $0 {start|stop|restart|status}"
        exit 1
        ;;
esac
exit 0
`

// runitRunTemplate is verbatim per AI.md PART 25 "Service Templates" >
// runit (Linux) "run script".
const runitRunTemplate = `#!/bin/sh
exec /usr/local/bin/{project_name} 2>&1
`

// runitLogRunTemplate is verbatim per AI.md PART 25 "Service Templates" >
// runit (Linux) "log/run script".
const runitLogRunTemplate = `#!/bin/sh
exec svlogd -tt /var/log/{internal_org}/{internal_name}
`

// rcdTemplate is verbatim per AI.md PART 25 "Service Templates" >
// rc.d (FreeBSD).
const rcdTemplate = `#!/bin/sh

# PROVIDE: {internal_name}
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name="{internal_name}"
rcvar="{internal_name}_enable"
command="/usr/local/bin/{project_name}"

load_rc_config $name
run_rc_command "$1"
`

// launchdTemplate is verbatim per AI.md PART 25 "Service Templates" >
// launchd (macOS).
const launchdTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{plist_name}</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/{project_name}</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/{internal_org}/{internal_name}/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/{internal_org}/{internal_name}/stderr.log</string>
</dict>
</plist>
`

// render substitutes every {placeholder} in tpl with the matching Context
// field. Placeholders are matched literally (no text/template parsing), so
// shell variables like $NAME or $PIDFILE in the sysvinit/OpenRC/rc.d
// templates are left untouched.
func render(tpl string, c Context) string {
	replacer := strings.NewReplacer(
		"{project_name}", c.ProjectName,
		"{project_org}", c.ProjectOrg,
		"{internal_org}", c.InternalOrg,
		"{internal_name}", c.InternalName,
		"{app_name}", c.appName(),
		"{plist_name}", c.plistName(),
	)
	return replacer.Replace(tpl)
}

// RenderSystemd renders the systemd unit file for c.
func RenderSystemd(c Context) string { return render(systemdTemplate, c) }

// RenderOpenRC renders the OpenRC init script for c.
func RenderOpenRC(c Context) string { return render(openrcTemplate, c) }

// RenderSysVinit renders the SysVinit init script for c.
func RenderSysVinit(c Context) string { return render(sysvinitTemplate, c) }

// RenderRunitRun renders runit's run script for c.
func RenderRunitRun(c Context) string { return render(runitRunTemplate, c) }

// RenderRunitLogRun renders runit's log/run script for c.
func RenderRunitLogRun(c Context) string { return render(runitLogRunTemplate, c) }

// RenderRcd renders the FreeBSD rc.d script for c.
func RenderRcd(c Context) string { return render(rcdTemplate, c) }

// RenderLaunchd renders the launchd plist for c.
func RenderLaunchd(c Context) string { return render(launchdTemplate, c) }
