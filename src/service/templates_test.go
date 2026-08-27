package service

import "testing"

func testContext() Context {
	return Context{
		ProjectName:  "redxt",
		ProjectOrg:   "webappsgo",
		InternalOrg:  "webappsgo",
		InternalName: "redxt",
		AppName:      "redxt service",
		BinaryPath:   "/usr/local/bin/redxt",
		ConfigDir:    "/etc/webappsgo/redxt",
		DataDir:      "/var/lib/webappsgo/redxt",
		CacheDir:     "/var/cache/webappsgo/redxt",
		LogDir:       "/var/log/webappsgo/redxt",
		BackupDir:    "/var/backups/webappsgo/redxt",
		PIDFile:      "/var/run/webappsgo/redxt.pid",
	}
}

func TestRenderSystemd(t *testing.T) {
	got := RenderSystemd(testContext())
	want := `[Unit]
Description=redxt service
Documentation=https://webappsgo.github.io/redxt
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/redxt
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

# Security hardening (binary drops privileges after port binding)
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/etc/webappsgo/redxt
ReadWritePaths=/var/lib/webappsgo/redxt
ReadWritePaths=/var/cache/webappsgo/redxt
ReadWritePaths=/var/log/webappsgo/redxt

[Install]
WantedBy=multi-user.target
`
	if got != want {
		t.Errorf("RenderSystemd mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderOpenRC(t *testing.T) {
	got := RenderOpenRC(testContext())
	want := "#!/sbin/openrc-run\n" +
		"# Service identity comes from redxt so config_dir/data_dir paths stay\n" +
		"# stable across binary renames (see PART 0 → \"Why `redxt` exists separately from `redxt`\").\n" +
		"\n" +
		"name=\"redxt\"\n" +
		"description=\"redxt service\"\n" +
		"# actual binary (may differ from redxt after rename)\n" +
		"command=\"/usr/local/bin/redxt\"\n" +
		"command_args=\"\"\n" +
		"command_user=\"redxt:redxt\"\n" +
		"pidfile=\"/var/run/webappsgo/redxt.pid\"\n" +
		"command_background=true\n" +
		"output_log=\"/var/log/webappsgo/redxt/server.log\"\n" +
		"error_log=\"/var/log/webappsgo/redxt/error.log\"\n" +
		"\n" +
		"depend() {\n" +
		"    need net\n" +
		"    after firewall\n" +
		"    use dns logger\n" +
		"}\n" +
		"\n" +
		"start_pre() {\n" +
		"    checkpath -d -m 0755 -o redxt:redxt /var/run/webappsgo\n" +
		"    checkpath -d -m 0755 -o redxt:redxt /var/log/webappsgo/redxt\n" +
		"}\n"
	if got != want {
		t.Errorf("RenderOpenRC mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderSysVinit(t *testing.T) {
	got := RenderSysVinit(testContext())
	want := `#!/bin/sh
### BEGIN INIT INFO
# Provides:          redxt
# Required-Start:    $network $remote_fs $syslog
# Required-Stop:     $network $remote_fs $syslog
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: redxt service
# Description:       redxt service daemon
### END INIT INFO

NAME=redxt
DAEMON=/usr/local/bin/redxt
DAEMON_USER=redxt
PIDFILE=/var/run/webappsgo/redxt.pid
LOGFILE=/var/log/webappsgo/redxt/server.log

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
	if got != want {
		t.Errorf("RenderSysVinit mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderRunitRun(t *testing.T) {
	got := RenderRunitRun(testContext())
	want := "#!/bin/sh\nexec /usr/local/bin/redxt 2>&1\n"
	if got != want {
		t.Errorf("RenderRunitRun mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderRunitLogRun(t *testing.T) {
	got := RenderRunitLogRun(testContext())
	want := "#!/bin/sh\nexec svlogd -tt /var/log/webappsgo/redxt\n"
	if got != want {
		t.Errorf("RenderRunitLogRun mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderRcd(t *testing.T) {
	got := RenderRcd(testContext())
	want := `#!/bin/sh

# PROVIDE: redxt
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name="redxt"
rcvar="redxt_enable"
command="/usr/local/bin/redxt"

load_rc_config $name
run_rc_command "$1"
`
	if got != want {
		t.Errorf("RenderRcd mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderLaunchd(t *testing.T) {
	got := RenderLaunchd(testContext())
	want := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>org.webappsgo.redxt</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/redxt</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/webappsgo/redxt/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/webappsgo/redxt/stderr.log</string>
</dict>
</plist>
`
	if got != want {
		t.Errorf("RenderLaunchd mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestContextAppNameFallsBackToProjectName(t *testing.T) {
	c := testContext()
	c.AppName = ""
	if got := c.appName(); got != "redxt" {
		t.Errorf("got %q, want %q", got, "redxt")
	}
}

func TestContextPlistName(t *testing.T) {
	c := testContext()
	if got := c.plistName(); got != "org.webappsgo.redxt" {
		t.Errorf("got %q, want %q", got, "org.webappsgo.redxt")
	}
}
