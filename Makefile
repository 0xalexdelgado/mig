# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at http://mozilla.org/MPL/2.0/.

BUILDREF	:= $(shell git log --pretty=format:'%h' -n 1)
BUILDDATE	:= $(shell date +%Y%m%d)
BUILDENV	:= dev
BUILDREV	:= $(BUILDDATE)+$(BUILDREF).$(BUILDENV)

# Supported OSes: linux darwin windows
# Supported ARCHes: 386 amd64
OS			:= linux
ARCH		:= amd64

ifeq ($(ARCH),amd64)
	FPMARCH := x86_64
endif
ifeq ($(ARCH),386)
	FPMARCH := i386
endif
ifeq ($(OS),windows)
	BINSUFFIX	:= ".exe"
else
	BINSUFFIX	:= ""
endif
PREFIX		:= /usr/local/
DESTDIR		:= /
BINDIR		:= bin/$(OS)/$(ARCH)
AGTCONF		:= conf/mig-agent-conf.go
AVAILMODS	:= conf/available_modules.go

GCC			:= gcc
CFLAGS		:=
LDFLAGS		:=
GOOPTS		:=
GO 			:= GOPATH=$(shell go env GOROOT)/bin:$(shell pwd) GOOS=$(OS) GOARCH=$(ARCH) go
GOGETTER	:= GOPATH=$(shell pwd) GOOS=$(OS) GOARCH=$(ARCH) go get -u
GOLDFLAGS	:= -ldflags "-X main.version $(BUILDREV)"
GOCFLAGS	:=
MKDIR		:= mkdir
INSTALL		:= install


all: mig-agent mig-scheduler mig-api mig-cmd mig-console mig-action-generator mig-action-verifier

mig-agent:
	echo building mig-agent for $(OS)/$(ARCH)
	if [ ! -r $(AGTCONF) ]; then echo "$(AGTCONF) configuration file is missing" ; exit 1; fi
	cp $(AGTCONF) src/mig/agent/configuration.go
	if [ ! -r $(AVAILMODS) ]; then echo "$(AGTCONF) configuration file is missing" ; exit 1; fi
	cp $(AVAILMODS) src/mig/agent/available_modules.go
	$(MKDIR) -p $(BINDIR)
	$(GO) build $(GOOPTS) -o $(BINDIR)/mig-agent-$(BUILDREV)$(BINSUFFIX) $(GOLDFLAGS) mig/agent
	ln -fs "$$(pwd)/$(BINDIR)/mig-agent-$(BUILDREV)$(BINSUFFIX)" "$$(pwd)/$(BINDIR)/mig-agent-latest"
	[ -x "$(BINDIR)/mig-agent-$(BUILDREV)$(BINSUFFIX)" ] && echo SUCCESS && exit 0

mig-scheduler:
	$(MKDIR) -p $(BINDIR)
	$(GO) build $(GOOPTS) -o $(BINDIR)/mig-scheduler $(GOLDFLAGS) mig/scheduler

mig-api:
	$(MKDIR) -p $(BINDIR)
	$(GO) build $(GOOPTS) -o $(BINDIR)/mig-api $(GOLDFLAGS) mig/api

mig-action-generator:
	$(MKDIR) -p $(BINDIR)
	$(GO) build $(GOOPTS) -o $(BINDIR)/mig-action-generator $(GOLDFLAGS) mig/client/generator

filechecker-convert:
	$(MKDIR) -p $(BINDIR)
	$(GO) build $(GOOPTS) -o $(BINDIR)/filechecker-convertv1tov2 $(GOLDFLAGS) mig/modules/filechecker/convert

mig-action-verifier:
	$(MKDIR) -p $(BINDIR)
	$(GO) build $(GOOPTS) -o $(BINDIR)/mig-action-verifier $(GOLDFLAGS) mig/client/verifier

mig-console:
	if [ ! -r $(AVAILMODS) ]; then echo "$(AGTCONF) configuration file is missing" ; exit 1; fi
	cp $(AVAILMODS) src/mig/client/console/available_modules.go
	$(MKDIR) -p $(BINDIR)
	$(GO) build $(GOOPTS) -o $(BINDIR)/mig-console $(GOLDFLAGS) mig/client/console

mig-cmd:
	if [ ! -r $(AVAILMODS) ]; then echo "$(AGTCONF) configuration file is missing" ; exit 1; fi
	cp $(AVAILMODS) src/mig/client/cmd/available_modules.go
	$(MKDIR) -p $(BINDIR)
	$(GO) build $(GOOPTS) -o $(BINDIR)/mig-$(OS)$(ARCH) $(GOLDFLAGS) mig/client/cmd
	ln -fs "$$(pwd)/$(BINDIR)/mig-$(OS)$(ARCH)" "$$(pwd)/$(BINDIR)/mig"

mig-agentsearch:
	$(MKDIR) -p $(BINDIR)
	$(GO) build $(GOOPTS) -o $(BINDIR)/mig-agentsearch $(GOLDFLAGS) mig/client/cmd/agentsearch

go_get_deps_into_system:
	make GOGETTER="go get -u" go_get_deps

go_get_deps:
	$(GOGETTER) code.google.com/p/go.crypto/openpgp
	$(GOGETTER) code.google.com/p/go.crypto/sha3
	$(GOGETTER) github.com/streadway/amqp
	$(GOGETTER) github.com/lib/pq
	$(GOGETTER) github.com/howeyc/fsnotify
	$(GOGETTER) code.google.com/p/gcfg
	$(GOGETTER) github.com/gorilla/mux
	$(GOGETTER) github.com/jvehent/cljs
	$(GOGETTER) bitbucket.org/kardianos/osext
	$(GOGETTER) github.com/jvehent/service-go
	$(GOGETTER) camlistore.org/pkg/misc/gpgagent
	$(GOGETTER) camlistore.org/pkg/misc/pinentry
	$(GOGETTER) github.com/ccding/go-stun/stun
ifeq ($(OS),windows)
	$(GOGETTER) code.google.com/p/winsvc/eventlog
endif
	$(GOGETTER) github.com/bobappleyard/readline
ifeq ($(OS),darwin)
	echo 'make sure that you have readline installed via {port,brew} install readline'
endif

install: mig-agent mig-scheduler
	$(INSTALL) -D -m 0755 $(BINDIR)/mig-agent $(DESTDIR)$(PREFIX)/sbin/mig-agent
	$(INSTALL) -D -m 0755 $(BINDIR)/mig-scheduler $(DESTDIR)$(PREFIX)/sbin/mig-scheduler
	$(INSTALL) -D -m 0755 $(BINDIR)/mig_action-generator $(DESTDIR)$(PREFIX)/bin/mig_action-generator
	$(INSTALL) -D -m 0640 mig.cfg $(DESTDIR)$(PREFIX)/etc/mig/mig.cfg
	$(MKDIR) -p $(DESTDIR)$(PREFIX)/var/cache/mig

rpm: rpm-agent rpm-scheduler

rpm-agent: mig-agent
# Bonus FPM options
#       --rpm-digest sha512 --rpm-sign
	rm -fr tmp
	$(INSTALL) -D -m 0755 $(BINDIR)/mig-agent-$(BUILDREV) tmp/sbin/mig-agent-$(BUILDREV)
	$(MKDIR) -p tmp/var/run/mig
	make agent-install-script
	make agent-remove-script
	fpm -C tmp -n mig-agent --license GPL --vendor mozilla --description "Mozilla InvestiGator Agent" \
		-m "Mozilla OpSec" --url http://mig.mozilla.org --architecture $(FPMARCH) -v $(BUILDREV) \
		--after-remove tmp/agent_remove.sh --after-install tmp/agent_install.sh --after-upgrade tmp/agent_install.sh \
		-s dir -t rpm .

deb-agent: mig-agent
	rm -fr tmp
	$(INSTALL) -D -m 0755 $(BINDIR)/mig-agent-$(BUILDREV) tmp/sbin/mig-agent-$(BUILDREV)
	$(MKDIR) -p tmp/var/run/mig
	make agent-install-script
	make agent-remove-script
	fpm -C tmp -n mig-agent --license GPL --vendor mozilla --description "Mozilla InvestiGator Agent" \
		-m "Mozilla OpSec" --url http://mig.mozilla.org --architecture $(FPMARCH) -v $(BUILDREV) \
		--after-remove tmp/agent_remove.sh --after-install tmp/agent_install.sh --after-upgrade tmp/agent_install.sh \
		-s dir -t deb .

osxpkg-agent: mig-agent
	rm -fr tmp
	mkdir 'tmp' 'tmp/sbin'
	$(INSTALL) -m 0755 $(BINDIR)/mig-agent-$(BUILDREV) tmp/sbin/mig-agent-$(BUILDREV)
	$(MKDIR) -p tmp/var/run/mig
	make agent-install-script
	make agent-remove-script
	fpm -C tmp -n mig-agent --license GPL --vendor mozilla --description "Mozilla InvestiGator Agent" \
		-m "Mozilla OpSec" --url http://mig.mozilla.org --architecture $(FPMARCH) -v $(BUILDREV) \
		--after-install tmp/agent_install.sh \
		-s dir -t osxpkg --osxpkg-identifier-prefix org.mozilla.mig .

agent-install-script:
	echo '#!/bin/sh'																				> tmp/agent_install.sh
	echo '[ -x "$$(which service)" ] && service mig-agent stop'										>> tmp/agent_install.sh
	echo '[ -x "$$(which initctl)" ] && initctl stop mig-agent'										>> tmp/agent_install.sh
	echo '[ -x "$$(which launchctl)" ] && launchctl unload /Library/LaunchDaemons/mig-agent.plist'	>> tmp/agent_install.sh
	echo '/sbin/mig-agent -q=pid 2>&1 1>/dev/null && kill $$(/sbin/mig-agent -q=pid)'				>> tmp/agent_install.sh
	echo 'echo deploying /sbin/mig-agent-$(BUILDREV) linked to /sbin/mig-agent'						>> tmp/agent_install.sh
	echo 'chmod 500 /sbin/mig-agent-$(BUILDREV)'													>> tmp/agent_install.sh
	echo 'chown root:root /sbin/mig-agent-$(BUILDREV)'												>> tmp/agent_install.sh
	echo 'rm /sbin/mig-agent; ln -s /sbin/mig-agent-$(BUILDREV) /sbin/mig-agent'					>> tmp/agent_install.sh
	echo '/sbin/mig-agent-$(BUILDREV)'																>> tmp/agent_install.sh
	chmod 0755 tmp/agent_install.sh

agent-remove-script:
	echo '#!/bin/sh'																				> tmp/agent_remove.sh
	echo 'echo shutting down running instances of mig-agent'										>> tmp/agent_remove.sh
	echo '[ -x "$$(which service)" ] && service mig-agent stop'										>> tmp/agent_remove.sh
	echo '[ -x "$$(which initctl)" ] && initctl stop mig-agent'										>> tmp/agent_remove.sh
	echo '[ -x "$$(which launchctl)" ] && launchctl unload /Library/LaunchDaemons/mig-agent.plist'	>> tmp/agent_remove.sh
	echo '/sbin/mig-agent -q=pid 2>&1 1>/dev/null && kill $$(/sbin/mig-agent -q=pid)'				>> tmp/agent_remove.sh
	echo 'rm -f "$$(readlink /sbin/mig-agent)" "/sbin/mig-agent"'									>> tmp/agent_remove.sh
	echo '[ -e "/etc/cron.d/mig-agent" ] && rm -f "/etc/cron.d/mig-agent"'							>> tmp/agent_remove.sh
	chmod 0755 tmp/agent_remove.sh

agent-cron:
	mkdir -p tmp/etc/cron.d/
	echo 'PATH="/usr/local/sbin:/usr/sbin:/sbin:/usr/local/bin:/usr/bin:/bin"'			> tmp/etc/cron.d/mig-agent
	echo 'SHELL=/bin/bash'																>> tmp/etc/cron.d/mig-agent
	echo 'MAILTO=""'																	>> tmp/etc/cron.d/mig-agent
	echo '*/10 * * * * root /sbin/mig-agent -q=pid 2>&1 1>/dev/null || /sbin/mig-agent' >> tmp/etc/cron.d/mig-agent
	chmod 0644 tmp/etc/cron.d/mig-agent

rpm-scheduler: mig-scheduler
	rm -rf tmp
	$(INSTALL) -D -m 0755 $(BINDIR)/mig-scheduler tmp/usr/bin/mig-scheduler
	$(INSTALL) -D -m 0640 conf/mig-scheduler.cfg.inc tmp/etc/mig/mig-scheduler.cfg
	$(MKDIR) -p tmp/var/cache/mig
	fpm -C tmp -n mig-scheduler --license GPL --vendor mozilla --description "Mozilla InvestiGator Scheduler" \
		-m "Mozilla OpSec" --url http://mig.mozilla.org --architecture $(FPMARCH) -v $(BUILDREV) -s dir -t rpm .

rpm-api: mig-api
	rm -rf tmp
	$(INSTALL) -D -m 0755 $(BINDIR)/mig-api tmp/usr/bin/mig-api
	$(INSTALL) -D -m 0640 conf/mig-api.cfg.inc tmp/etc/mig/mig-api.cfg
	$(MKDIR) -p tmp/var/cache/mig
	fpm -C tmp -n mig-api --license GPL --vendor mozilla --description "Mozilla InvestiGator API" \
		-m "Mozilla OpSec" --url http://mig.mozilla.org --architecture $(FPMARCH) -v $(BUILDREV) -s dir -t rpm .

test: mig-agent
	$(BINDIR)/mig-agent-latest -m=file '{"searches": {"shouldmatch": {"names": ["^root"],"sizes": ["<10m"],"options": {"matchall": true},"paths": ["/etc/passwd"]},"shouldnotmatch": {"options": {"maxdepth": 1},"paths": ["/tmp"],"contents": ["should not match"]}}}'

clean-agent:
	find bin/ -name mig-agent* -exec rm {} \;
	rm -rf packages
	rm -rf tmp

clean: clean-agent
	rm -rf bin
	rm -rf tmp
	find src/ -maxdepth 1 -mindepth 1 ! -name mig -exec rm -rf {} \;

.PHONY: clean clean-all clean-agent doc go_get_deps_into_system mig-agent-386 mig-agent-amd64 agent-install-script agent-cron
