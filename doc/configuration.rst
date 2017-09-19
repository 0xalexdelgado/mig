Mozilla InvestiGator Deployment and Configuration Documentation
===============================================================

.. sectnum::
.. contents:: Table of Contents

This document describes the steps to build and configure a MIG platform. Here we
go into specific details on manual configuration of a simple environment. For another
example, see the MIG `Docker image`_ which executes similar steps.

.. _`Docker image`: ../Dockerfile

MIG has 6 major components.

* Scheduler
* API
* Postgres
* RabbitMQ
* Agents
* Investigator tools (command line clients)

The Postgres database and RabbitMQ relay are external dependencies, and while
this document shows one way of deploying them, you are free to use your own method.

No binary packages are provided for MIG, so to try it you will need to build the
software yourself or make use of the docker image.

A complete environment should be configured in the following order:

1. Retrieve the source and prepare your build environment
2. Deploy the Postgres database
3. Create a PKI
4. Deploy the RabbitMQ relay
5. Build, configure and deploy the scheduler
6. Build, configure and deploy the API
7. Build the clients and create an investigator
8. Configure and deploy agents

Prepare a build environment
---------------------------

Install the latest version of go. Usually you can do this using your operating system's
package manager (e.g., ``apt-get install golang`` on Ubuntu), or you can also fetch and
install it directly at https://golang.org/.

.. code:: bash

        $ go version
        go version go1.8 linux/amd64

As with any go setup, make sure your GOPATH is exported, for example by setting
it to ``$HOME/go``

.. code:: bash

        $ export GOPATH="$HOME/go"
        $ mkdir $GOPATH

Then retrieve MIG's source code using go get:

.. code:: bash

        $ go get mig.ninja/mig

``go get`` will place MIG under ``$GOPATH/src/mig.ninja/mig``. If you want you can run
``make test`` under this directory to verify the tests execute and ensure your go environment
is setup correctly.

.. code:: bash

        $ make test
        GOOS=linux GOARCH=amd64 GO15VENDOREXPERIMENT=1 go test mig.ninja/mig/modules/
        ok      mig.ninja/mig/modules   0.103s
        GOOS=linux GOARCH=amd64 GO15VENDOREXPERIMENT=1 go test mig.ninja/mig/modules/agentdestroy
        ok      mig.ninja/mig/modules/agentdestroy      0.003s
        GOOS=linux GOARCH=amd64 GO15VENDOREXPERIMENT=1 go test mig.ninja/mig/modules/example
        ok      mig.ninja/mig/modules/example   0.003s
        GOOS=linux GOARCH=amd64 GO15VENDOREXPERIMENT=1 go test mig.ninja/mig/modules/examplepersist
        ok      mig.ninja/mig/modules/examplepersist    0.002s
        GOOS=linux GOARCH=amd64 GO15VENDOREXPERIMENT=1 go test mig.ninja/mig/modules/file
        ok      mig.ninja/mig/modules/file      0.081s
        GOOS=linux GOARCH=amd64 GO15VENDOREXPERIMENT=1 go test mig.ninja/mig/modules/fswatch
        ok      mig.ninja/mig/modules/fswatch   0.003s
        ...

Deploy the Postgres database
----------------------------

Install Postgres 9.5+ on a server, or you can also use something like Amazon RDS. To get the
Postgres database ready to use with MIG, we will need to create a few roles and install the
database schema. Note this guide shows examples assuming Postgres running on the local server,
for a different configuration adjust your commands accordingly.

The API and scheduler need to connect to the database over the TCP socket; you might need to
adjust the default ``pg_hba.conf`` to permit these connections, for example by adding a line
as follows:

.. code::

        host all all 127.0.0.1/32 password

Once the database is ready to be configured, start by adding a few roles. Adjust the commands
below to set the database user passwords you want, and note them for later.

.. code:: bash

        $ sudo -u postgres psql -c 'CREATE ROLE migadmin;'
        $ sudo -u postgres psql -c "ALTER ROLE migadmin WITH NOSUPERUSER INHERIT NOCREATEROLE NOCREATEDB LOGIN PASSWORD 'userpass';"
        $ sudo -u postgres psql -c 'CREATE ROLE migapi;'
        $ sudo -u postgres psql -c "ALTER ROLE migapi WITH NOSUPERUSER INHERIT NOCREATEROLE NOCREATEDB LOGIN PASSWORD 'userpass';"
        $ sudo -u postgres psql -c 'CREATE ROLE migscheduler;'
        $ sudo -u postgres psql -c "ALTER ROLE migscheduler WITH NOSUPERUSER INHERIT NOCREATEROLE NOCREATEDB LOGIN PASSWORD 'userpass';"

Next create the database and install the schema:

.. code:: bash

        $ sudo -u postgres psql -c 'CREATE DATABASE mig;'
        $ cd $GOPATH/src/mig.ninja/mig
        $ sudo -u postgres psql -f database/schema.sql mig

Create a PKI
------------

With a standard MIG installation, the agents connect to the relay (RabbitMQ) over
a TLS protected connection. Certificate validation occurs against the RabbitMQ server
certificate, and in addition client certificates are validated by RabbitMQ in order
to add an extra layer to prevent unauthorized connections to the public AMQP endpoint.

Skip this step if you want to reuse an existing PKI. MIG will need a server
certificate for RabbitMQ, and client certificates for agents and the scheduler.

You can either create the PKI yourself using something like the ``openssl`` command,
or alternatively take a look at ``tools/create_mig_ca.sh`` which can run these
commands for you. In this example we will use the script.

Create a new directory that will hold the CA, copy the script to it, and run it.
The script will prompt for one piece of information: the public DNS of the
RabbitMQ relay. It's important that you set this to the correct value to allow
AMQP clients to validate the RabbitMQ certificate correctly.

.. code:: bash

	$ mkdir migca
	$ cd migca
	$ cp $GOPATH/src/mig.ninja/mig/tools/create_mig_ca.sh .
	$ bash create_mig_ca.sh
	[...]
	enter the public dns name of the rabbitmq server agents will connect to> mymigrelay.example.net
	[...]
	$ ls -l
	total 76
	-rw-r--r-- 1 julien julien 5163 Sep  9 00:06 agent.crt
	-rw-r--r-- 1 julien julien 1033 Sep  9 00:06 agent.csr
	-rw-r--r-- 1 julien julien 1704 Sep  9 00:06 agent.key
	drwxr-xr-x 3 julien julien 4096 Sep  9 00:06 ca
	-rw-r--r-- 1 julien julien 3608 Sep  9 00:06 create_mig_ca.sh
	-rw-r--r-- 1 julien julien 2292 Sep  9 00:06 openssl.cnf
	-rw-r--r-- 1 julien julien 5161 Sep  9 00:06 rabbitmq.crt
	-rw-r--r-- 1 julien julien 1029 Sep  9 00:06 rabbitmq.csr
	-rw-r--r-- 1 julien julien 1704 Sep  9 00:06 rabbitmq.key
	-rw-r--r-- 1 julien julien 5183 Sep  9 00:06 scheduler.crt
	-rw-r--r-- 1 julien julien 1045 Sep  9 00:06 scheduler.csr
	-rw-r--r-- 1 julien julien 1704 Sep  9 00:06 scheduler.key

Deploy the RabbitMQ relay
-------------------------

Installation
~~~~~~~~~~~~

Install the RabbitMQ server from your distribution's packaging system. If your
distribution does not provide a RabbitMQ package, install ``erlang`` from ``yum`` or
``apt``, and then install RabbitMQ using the packages from http://www.rabbitmq.com/.

RabbitMQ Configuration
~~~~~~~~~~~~~~~~~~~~~~

To configure RabbitMQ, we will need to add users to the relay and add permissions.

We will need a user for the scheduler, as the scheduler talks to the relay to send
actions to the agents and receive results. We will also want a user that the agents
will use to connect to the relay. We will also add a general admin account that can
be used for example with the RabbitMQ administration interface if desired.

The following commands can be used to configure RabbitMQ, adjust the commands below
as required to set the passwords you want for each account. Note the passwords as
we will need them later.

.. code:: bash

        $ sudo rabbitmqctl add_user admin adminpass
        $ sudo rabbitmqctl set_user_tags admin administrator
        $ sudo rabbitmqctl delete_user guest
        $ sudo rabbitmqctl add_vhost mig
        $ sudo rabbitmqctl add_user scheduler schedulerpass
        $ sudo rabbitmqctl set_permissions -p mig scheduler \
                '^(toagents|toschedulers|toworkers|mig\.agt\..*)$' \
                '^(toagents|toworkers|mig\.agt\.(heartbeats|results))$' \
                '^(toagents|toschedulers|toworkers|mig\.agt\.(heartbeats|results))$'
        $ sudo rabbitmqctl add_user agent agentpass
        $ sudo rabbitmqctl set_permissions -p mig agent \
                '^mig\.agt\..*$' \
                '^(toschedulers|mig\.agt\..*)$' \
                '^(toagents|mig\.agt\..*)$'
        $ sudo rabbitmqctl add_user worker workerpass
        $ sudo rabbitmqctl set_permissions -p mig worker \
                '^migevent\..*$' \
                '^migevent(|\..*)$' \
                '^(toworkers|migevent\..*)$'
        $ sudo service rabbitmq-server restart

Now that we have added users, we will want to enable AMQPS for SSL/TLS connections
to the relay.

.. code:: bash

        $ cd ~/migca
        $ sudo cp rabbitmq.crt /etc/rabbitmq/rabbitmq.crt
        $ sudo cp rabbitmq.key /etc/rabbitmq/rabbitmq.key
        $ sudo cp ca/ca.crt /etc/rabbitmq/ca.crt

Now edit the default RabbitMQ configuration to enable TLS, and you should have something
like this:

.. code::

	[
	  {rabbit, [
	         {ssl_listeners, [5671]},
                 {ssl_options, [{cacertfile,            "/etc/rabbitmq/ca.crt"},
                                {certfile,              "/etc/rabbitmq/rabbitmq.crt"},
                                {keyfile,               "/etc/rabbitmq/rabbitmq.key"},
                                {verify,                verify_peer},
                                {fail_if_no_peer_cert,  true},
                                {versions, ['tlsv1.2', 'tlsv1.1']},
                                {ciphers,  [{dhe_rsa,aes_256_cbc,sha256},
                                            {dhe_rsa,aes_128_cbc,sha256},
                                            {dhe_rsa,aes_256_cbc,sha},
                                            {rsa,aes_256_cbc,sha256},
                                            {rsa,aes_128_cbc,sha256},
                                            {rsa,aes_256_cbc,sha}]}
                 ]}
	  ]}
	].

Now, restart RabbitMQ.

.. code:: bash

        $ sudo service rabbitmq-server restart

You should have RabbitMQ listening on port ``5671`` now.

.. code:: bash

	$ netstat -taupen|grep 5671
	tcp6	0	0	:::5671		:::*	LISTEN	110	658831	11467/beam.smp  

Scheduler Configuration
-----------------------

Now that the relay and database are online, we can deploy the MIG scheduler. Start
by building and installing it, we will run it from ``/opt/mig`` in this example.

.. code:: bash

        $ sudo mkdir -p /opt/mig/bin
        $ cd $GOPATH/src/mig.ninja/mig
        $ make mig-scheduler
        $ sudo cp bin/linux/amd64/mig-scheduler /opt/mig/bin/mig-scheduler

The scheduler needs a configuration file, you can start with the
`default scheduler configuration file`_.

.. _`default scheduler configuration file`: ../conf/scheduler.cfg.inc

.. code:: bash

        $ sudo mkdir -p /etc/mig
        $ sudo cp conf/scheduler.cfg.inc /etc/mig/scheduler.cfg

The scheduler has several options, which are not documented here. The primary sections
you will want to modify are the ``mq`` section and the ``postgres`` section. These sections
should be updated with information to connect to the database and relay using the users and
passwords created for the scheduler in the previous steps.

In the ``mq`` section, you will also want to make sure ``usetls`` is enabled. Set the
certificate and key paths to point to the scheduler certificate information under
``/etc/mig``, and copy the files we created in the PKI step.

.. code:: bash

        $ cd ~/migca
        $ sudo cp scheduler.crt /etc/mig
        $ sudo cp scheduler.key /etc/mig
        $ sudo cp ca/ca.key /etc/mig

We can now try running the scheduler in the foreground to validate it is working correctly.

.. code:: bash

	# /opt/mig/bin/mig-scheduler 
	Initializing Scheduler context...OK
	2015/09/09 04:25:47 - - - [debug] leaving initChannels()
	2015/09/09 04:25:47 - - - [debug] leaving initDirectories()
	2015/09/09 04:25:47 - - - [info] Database connection opened
	2015/09/09 04:25:47 - - - [debug] leaving initDB()
	2015/09/09 04:25:47 - - - [info] AMQP connection opened
	2015/09/09 04:25:47 - - - [debug] leaving initRelay()
	2015/09/09 04:25:47 - - - [debug] leaving makeSecring()
	2015/09/09 04:25:47 - - - [info] no key found in database. generating a private key for user migscheduler
	2015/09/09 04:25:47 - - - [info] created migscheduler identity with ID %!d(float64=1) and key ID A8E1ED58512FCD9876DBEA4FEA513B95032D9932
	2015/09/09 04:25:47 - - - [debug] leaving makeSchedulerInvestigator()
	2015/09/09 04:25:47 - - - [debug] loaded scheduler private key from database
	2015/09/09 04:25:47 - - - [debug] leaving makeSecring()
	2015/09/09 04:25:47 - - - [info] Loaded scheduler investigator with key id A8E1ED58512FCD9876DBEA4FEA513B95032D9932
	2015/09/09 04:25:47 - - - [debug] leaving initSecring()
	2015/09/09 04:25:47 - - - [info] mig.ProcessLog() routine started
	2015/09/09 04:25:47 - - - [info] processNewAction() routine started
	2015/09/09 04:25:47 - - - [info] sendCommands() routine started
	2015/09/09 04:25:47 - - - [info] terminateCommand() routine started
	2015/09/09 04:25:47 - - - [info] updateAction() routine started
	2015/09/09 04:25:47 - - - [info] agents heartbeats listener initialized
	2015/09/09 04:25:47 - - - [debug] leaving startHeartbeatsListener()
	2015/09/09 04:25:47 - - - [info] agents heartbeats listener routine started
	2015/09/09 04:25:47 4883372310530 - - [info] agents results listener initialized
	2015/09/09 04:25:47 4883372310530 - - [debug] leaving startResultsListener()
	2015/09/09 04:25:47 - - - [info] agents results listener routine started
	2015/09/09 04:25:47 - - - [info] collector routine started
	2015/09/09 04:25:47 - - - [info] periodic routine started
	2015/09/09 04:25:47 - - - [info] queue cleanup routine started
	2015/09/09 04:25:47 - - - [info] killDupAgents() routine started
	2015/09/09 04:25:47 4883372310531 - - [debug] initiating spool inspection
	2015/09/09 04:25:47 4883372310532 - - [info] initiating periodic run
	2015/09/09 04:25:47 4883372310532 - - [debug] leaving cleanDir()
	2015/09/09 04:25:47 4883372310532 - - [debug] leaving cleanDir()
	2015/09/09 04:25:47 4883372310531 - - [debug] leaving loadNewActionsFromDB()
	2015/09/09 04:25:47 4883372310531 - - [debug] leaving loadNewActionsFromSpool()
	2015/09/09 04:25:47 4883372310531 - - [debug] leaving loadReturnedCommands()
	2015/09/09 04:25:47 4883372310531 - - [debug] leaving expireCommands()
	2015/09/09 04:25:47 4883372310531 - - [debug] leaving spoolInspection()
	2015/09/09 04:25:47 4883372310532 - - [debug] leaving markOfflineAgents()
	2015/09/09 04:25:47 4883372310533 - - [debug] QueuesCleanup(): found 0 offline endpoints between 2015-09-08 01:25:47.292598629 +0000 UTC and now
	2015/09/09 04:25:47 4883372310533 - - [info] QueuesCleanup(): done in 7.389363ms
	2015/09/09 04:25:47 4883372310533 - - [debug] leaving QueuesCleanup()
	2015/09/09 04:25:47 4883372310532 - - [debug] leaving markIdleAgents()
	2015/09/09 04:25:47 4883372310532 - - [debug] CountNewEndpoints() took 7.666476ms to run
	2015/09/09 04:25:47 4883372310532 - - [debug] CountIdleEndpoints() took 99.925426ms to run
	2015/09/09 04:25:47 4883372310532 - - [debug] SumIdleAgentsByVersion() took 99.972162ms to run
	2015/09/09 04:25:47 4883372310532 - - [debug] SumOnlineAgentsByVersion() took 100.037988ms to run
	2015/09/09 04:25:47 4883372310532 - - [debug] CountFlappingEndpoints() took 100.134112ms to run
	2015/09/09 04:25:47 4883372310532 - - [debug] CountOnlineEndpoints() took 99.976176ms to run
	2015/09/09 04:25:47 4883372310532 - - [debug] CountDoubleAgents() took 99.959133ms to run
	2015/09/09 04:25:47 4883372310532 - - [debug] CountDisappearedEndpoints() took 99.900215ms to run
	2015/09/09 04:25:47 4883372310532 - - [debug] leaving computeAgentsStats()
	2015/09/09 04:25:47 4883372310532 - - [debug] leaving detectMultiAgents()
	2015/09/09 04:25:47 4883372310532 - - [debug] leaving periodic()
	2015/09/09 04:25:47 4883372310532 - - [info] periodic run done in 110.647479ms

Assuming the default logging parameters in the configuration file were not changed, the scheduler
starts up and begins writing its log to stdout.  Among the debug logs, we can see that the scheduler
successfully connected to both Postgres and RabbitMQ. It detected that no scheduler key was
present in the database and created one with Key ID "A8E1ED58512FCD9876DBEA4FEA513B95032D9932".
It then proceeded to wait for work to do, waking up regularly to perform maintenance tasks.

The key the scheduler generated is used by the scheduler when it sends destruction orders to duplicate
agents. When the scheduler detects more than one agent running on the same host, it will request the
old agent stop. The scheduler signs this request with the key it created, so the agent will need to know
to trust this key. This is discussed later when we configure an agent.

In a production scenario, you will likely want to create a systemd unit to run the scheduler or some
other form of supervisor.

API configuration
-----------------

MIG's REST API is the interface between investigators and the rest of the
infrastructure. It is also accessed by agents to discover their public IP. Generally
speaking, agents communicate using the relay, and investigators access the agents
through the API.

The API needs to be deployed like a normal web application, preferably behind a
reverse proxy that handles TLS. The API does not handle TLS on it's own. You can use
something like an Amazon ELB in front of the API, or you can also use something
like Nginx.

For this documentation, we will assume that the API listens on its local IP,
which is 192.168.1.150, on port 51664, and the public endpoint of the API is
``api.mig.example.net``. We start by building the API and installing the
`default API configuration`_.

.. _`default API configuration`: ../conf/api.cfg.inc

.. code:: bash

        $ cd $GOPATH/src/mig.ninja/mig
        $ make mig-api
        $ sudo cp bin/linux/amd64/mig-api /opt/mig/bin/mig-api
        $ sudo cp conf/api.cfg.inc /etc/mig/api.cfg

Edit the configuration file and tweak it as desired. Most options can remain at
the default setting,  however there are a few we will want to change.

Edit the ``postgres`` section and configure this with the correct settings so
the API can connect to the database using the API user we create in a previous step.

You will also want to edit the local listening port, in our example we will set it
to port ``51664``. Set the ``host`` parameter to the URL corresponding with the
API, so in this example ``https://api.mig.example.net``.

You will also want to pay attention to the ``authentication`` section, specifically
the ``enabled`` parameter. This is initially off, and we will leave it off so we
can create our initial investigator in the system. Once we have setup our initial
investigator we will enable API authentication.

Ensure ``clientpublicip`` is set based on your environment. If clients are terminated
directly on the API, ``peer`` can be used. If a load balancer or other device terminates
connections from clients and adds the address to X-Forwarded-For, ``x-forwarded-for``
can be used. The integer trailing ``x-forwarded-for`` specifies the offset from the end
of the list of IPs in the header to use to extract the IP. For example,
x-forwarded-for:0 would get the last IP in a list in that header, x-forwarded-for:1
would get the second last, etc. Set this based on the number of forwarding devices
you have between the client and the API.

At this point the API is ready to go, and if desired a reverse proxy can be configured
in front of the API to enable TLS.

A sample Nginx reverse proxy configuration is shown below.

.. code::

	server {
		listen 443;
		ssl on;

		root /var/www;
		index index.html index.htm;
		server_name api.mig.example.net;
		client_max_body_size 200M;

		# certs sent to the client in SERVER HELLO are concatenated in ssl_certificate
		ssl_certificate        /etc/nginx/certs/api.mig.example.net.crt;
		ssl_certificate_key    /etc/nginx/certs/api.mig.example.net.key;
		ssl_session_timeout    5m;
		ssl_session_cache      shared:SSL:50m;

		# Diffie-Hellman parameter for DHE ciphersuites, recommended 2048 bits
		ssl_dhparam        /etc/nginx/certs/dhparam;

		# modern configuration. tweak to your needs.
		ssl_protocols TLSv1.1 TLSv1.2;
		ssl_ciphers 'ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-AES256-GCM-SHA384:DHE-RSA-AES128-GCM-SHA256:DHE-DSS-AES128-GCM-SHA256:kEDH+AESGCM:ECDHE-RSA-AES128-SHA256:ECDHE-ECDSA-AES128-SHA256:ECDHE-RSA-AES128-SHA:ECDHE-ECDSA-AES128-SHA:ECDHE-RSA-AES256-SHA384:ECDHE-ECDSA-AES256-SHA384:ECDHE-RSA-AES256-SHA:ECDHE-ECDSA-AES256-SHA:DHE-RSA-AES128-SHA256:DHE-RSA-AES128-SHA:DHE-DSS-AES128-SHA256:DHE-RSA-AES256-SHA256:DHE-DSS-AES256-SHA:DHE-RSA-AES256-SHA:!aNULL:!eNULL:!EXPORT:!DES:!RC4:!3DES:!MD5:!PSK';
		ssl_prefer_server_ciphers on;

		location /api/v1/ {
			proxy_set_header X-Forwarded-For $remote_addr;
			proxy_pass http://192.168.1.150:51664/api/v1/;
		}
	}

If you're going to enable HTTPS in front of the API, make sure to use a trusted
certificate. Agents don't connect to untrusted certificates. If you are setting up a test
environment and don't want to enable SSL/TLS, you can run Nginx in HTTP mode or just use
the API alone, however this configuration is not recommended.

We can now try running the API in the foreground to validate it is working correctly.

.. code:: bash

	# /opt/mig/bin/mig-api
        Initializing API context...OK
        2017/09/18 17:24:54 - - - [info] Database connection opened
        2017/09/18 17:24:54 - - - [debug] leaving initDB()
        2017/09/18 17:24:54 - - - [info] Context initialization done
        2017/09/18 17:24:54 - - - [info] Logger routine started
        2017/09/18 17:24:54 - - - [info] Starting HTTP handler

You can test that the API works properly by performing a request to the
dashboard endpoint. It should return a JSON document with all counters at zero,
since we don't have any agent connected yet. Note that we can do this, as authentication
in the API has not yet been enabled, normally this request would be rejected without
a valid signed token or API key.

.. code:: json

	$ curl https://api.mig.example.net/api/v1/dashboard | python -mjson.tool
	{
		"collection": {
			"version": "1.0",
			"href": "https://api.mig.example.net/api/v1/dashboard",
			"items": [
				{
					"href": "https://api.mig.example.net/api/v1/dashboard",
					"data": [
						{
							"name": "online agents",
							"value": 0
						},
						{
							"name": "online agents by version",
							"value": null
						},
						{
							"name": "online endpoints",
							"value": 0
						},
						{
							"name": "idle agents",
							"value": 0
						},
						{
							"name": "idle agents by version",
							"value": null
						},
						{
							"name": "idle endpoints",
							"value": 0
						},
						{
							"name": "new endpoints",
							"value": 0
						},
						{
							"name": "endpoints running 2 or more agents",
							"value": 0
						},
						{
							"name": "disappeared endpoints",
							"value": 0
						},
						{
							"name": "flapping endpoints",
							"value": 0
						}
					]
				}
			],
			"template": {},
			"error": {}
		}
	}

Build the clients and create an investigator
--------------------------------------------

MIG has multiple command line clients that can be used to interact with the API
and run investigations or view results. The two main clients are ``mig``, a
command line tool that can run investigations quickly, and ``mig-console``, a
readline console that can run investigations but also browse through past
investigations as well and manage investigators. We will use ``mig-console`` to
create our first investigator.

Here we will assume you already have GnuPG installed, and that you generate a
keypair for yourself (see the `doc on gnupg.org
<https://www.gnupg.org/gph/en/manual.html#AEN26>`_).
You should be able to access your PGP fingerprint using this command:

.. code:: bash

	$ gpg --fingerprint myinvestigator@example.net

	pub   2048R/3B763E8F 2013-04-30
	Key fingerprint = E608 92BB 9BD8 9A69 F759  A1A0 A3D6 5217 3B76 3E8F
	uid                  My Investigator <myinvestigator@example.net>
	sub   2048R/8026F39F 2013-04-30

Next, create the client configuration file in `$HOME/.migrc`. Below is a sample
you can reuse with your own values.

.. code::

	$ cat ~/.migrc
	[api]
		url = "https://api.mig.example.net/api/v1/"
	[gpg]
		home = "/home/myuser/.gnupg/"
		keyid = "E60892BB9BD89A69F759A1A0A3D652173B763E8F"
        [targets]
                macro = allonline:status='online'
                macro = idleandonline:status='online' OR status='idle'

The targets section is optional and provides the ability to specify
short forms of your own targeting strings. In the example above, 
``allonline`` or ``idleandonline`` could be used as target arguments when
running an investigation.

Make sure have the dev library of readline installed (``readline-devel`` on
RHEL/Fedora or ``libreadline-dev`` on Debian/Ubuntu), and built the command
line tools.

.. code::

	$ sudo apt-get install libreadline-dev
        $ cd $GOPATH/src/mig.ninja/mig
        $ make mig-cmd
        $ make mig-console
        $ bin/linux/amd64/mig-console

	## ##                                     _.---._     .---.
	# # # /-\ ---||  |    /\         __...---' .---. '---'-.   '.
	#   #|   | / ||  |   /--\    .-''__.--' _.'( | )'.  '.  '._ :
	#   # \_/ ---| \_ \_/    \ .'__-'_ .--'' ._'---'_.-.  '.   '-'.
		 ###                         ~ -._ -._''---. -.    '-._   '.
		  # |\ |\    /---------|          ~ -.._ _ _ _ ..-_ '.  '-._''--.._
		  # | \| \  / |- |__ | |                       -~ -._  '-.  -. '-._''--.._.--''.
		 ###|  \  \/  ---__| | |                            ~ ~-.__     -._  '-.__   '. '.
			  #####                                               ~~ ~---...__ _    ._ .' '.
			  #      /\  --- /-\ |--|----                                    ~  ~--.....--~
			  # ### /--\  | |   ||-\  //
			  #####/    \ |  \_/ |  \//__
	+------
	| Agents & Endpoints summary:
	| * 0 online agents on 0 endpoints
	| * 0 idle agents on 0 endpoints
	| * 0 endpoints are running 2 or more agents
	| * 0 endpoints appeared over the last 7 days
	| * 0 endpoints disappeared over the last 7 days
	| * 0 endpoints have been flapping
	| Online agents by version:
	| Idle agents by version:
	|
	| Latest Actions:
	| ----    ID      ---- + ----         Name         ---- + -Sent- + ----    Date     ---- + ---- Investigators ----
	+------

	Connected to https://api.mig.example.net/api/v1/. Exit with ctrl+d. Type help for help.
	mig>

The console will wait for input on the `mig>` prompt. Enter `help` if you want to
explore all the available functions. For now, we will only create a new investigator
in the database.

The investigator will be defined with its public key, so the first thing we
need to do is export our public key to a local file that can be given to the
console during the creation process.

.. code::

	$ gpg --export -a myinvestigator@example.net > /tmp/myinvestigator_pubkey.asc

Then in the console prompt, enter the following commands:

- ``create investigator``
- enter a name, such as ``Bob The Investigator``
- choose ``yes`` to make our first investigator an administrator
- choose ``yes`` to allow our first investigator to manage loaders
- choose ``yes`` to allow our first investigator to manage manifests
- choose ``yes`` to add a public PGP key for this new investigator
- enter the path to the public key `/tmp/myinvestigator_pubkey.asc`
- enter `y` to confirm the creation

Choosing to make the investigator an administrator permits user management and other 
administrative functions. The loader and manifest options we set to yes, but these are
only relevant if you are using ``mig-loader`` to automatically update agents. This is not
discussed in this guide, for more information see the `MIG loader`_ documentation.

.. _`MIG loader`: loader.rst

The console should display "Investigator 'Bob The Investigator' successfully
created with ID 2". We can view the details of this new investigator by entering
``investigator 2`` on the console prompt.

.. code::

        mig> investigator 2
        Entering investigator mode. Type exit or press ctrl+d to leave. help may help.
        Investigator 2 named 'Bob The Investigator'
        
        inv 2> details
        Investigator ID 2
        name         Bob The Investigator
        status       active
        permissions  Default,PermAdmin,PermLoader,PermManifest
        key id       E60892BB9BD89A69F759A1A0A3D652173B763E8F
        created      2015-09-09 09:53:28.989481 -0400 EDT
        modified     2015-09-09 09:53:28.989481 -0400 EDT
        api key set  false

Enable API Authentication
~~~~~~~~~~~~~~~~~~~~~~~~~

Now that we have an active investigator created, we can enable authentication
in the API. Go back to the API server and modify the configuration in
``/etc/mig/api.cfg``.

.. code::

	[authentication]
		# turn this on after initial setup, once you have at least
		# one investigator created
		enabled = on

Since the user we create in the previous step was created as an administrator, we can now
use this user to add other investigators to the system.

Reopen ``mig-console``, and you will see the investigator name in the API logs:

.. code::

	2015/09/09 13:56:09 4885615083520 - - [info] src=192.168.1.243,192.168.1.1 auth=[Bob The Investigator 2] GET HTTP/1.0 /api/v1/dashboard resp_code=200 resp_size=600 user-agent=MIG Client console-20150826+62ea662.dev

The server side of MIG has now been configured, and we can move on to configuring agents.

MIG loader Configuration
------------------------

At this point you will want to decide if you wish to use ``mig-loader`` to keep
your agents up to date on remote endpoints.

With mig-loader, instead of installing the agent on the systems you want to run
the agent on, you would install only mig-loader. mig-loader is a small binary
intended to be run from a periodic system such as cron. mig-loader will then
look after fetching the agent and installing it if it does not exist on the system,
and will look after upgrading the agent automatically if you want to publish new
agent updates. The upgrades can be controlled by a MIG administrator through the
MIG API and console tools.

For information on the loader, see the `mig-loader`_ documentation. If you wish to
use mig-loader, read the documentation to understand how the rest of this guide fits
into configuration with loader based deployment.

.. _`mig-loader`: loader.rst

Agent Configuration
-------------------

There are a couple different ways to configure the agent for your environment.
Historically, the agent had certain configuration values that were specified at
compile time in the agents built-in configuration (`configuration.go`_). Setting
values here is no longer required, so it is possible to deploy the agent using
entirely external configuration.

.. _`configuration.go`: ../mig-agent/configuration.go

You can choose to either:

* Edit the agent built-in configuration before you compile it
* Use a configuration file

The benefit of editing the configuration before compilation is you can deploy an
agent to a remote host by solely installing the agent binary. The drawback to this
method is, any changes to the configuration require recompiling the agent and
installing the new binary.

This guide will discuss the preferred method of using external configuration to
deploy the agent.

Compiling the agent with desired modules
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

When the agent is built, certain tags can be specified to control which modules
will be included with the agent. See the `documentation`_ included with the various
modules to decide which modules you want; in a lot of circumstances the default
module pack is sufficient.

.. _`documentation`: ../modules

To build with the default modules, no addition flags are required to ``make``.

.. code:: bash

        $ make mig-agent

The ``MODULETAGS`` parameter can be specified to include additional modules, or to
exclude the defaults. This example shows building the agent with the default modules,
in addition to the memory module.

.. code:: bash

        $ make MODULETAGS='modmemory' mig-agent

This example shows building the agent without the default module set, and only including
the file module and scribe module.

.. code:: bash

        $ make MODULETAGS='modnodefaults modfile modscribe' mig-agent

The ``MODULETAGS`` parameter just sets certain tags with the ``go build`` command to
control the inclusion of the modules. You can also do this with commands like ``go get``
or ``go install``.

.. code:: bash

        $ go install -tags 'modnodefaults modmemory' mig.ninja/mig/mig-agent

Install the agent configuration file
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

We can start with the default agent configuration template in `conf/mig-agent.cfg.inc`_.

.. _`conf/mig-agent.cfg.inc`: ../conf/mig-agent.cfg.inc

.. code:: bash

        $ sudo cp conf/mig-agent.cfg.inc /etc/mig/mig-agent.cfg

Update agent configuration
~~~~~~~~~~~~~~~~~~~~~~~~~~

TLS support between agents and RabbitMQ is optional, but strongly recommended.
To use TLS, we will use the certificates we created for the agent in the PKI step.
Copy the certificates into place in ``/etc/mig``.

.. code:: bash

        $ cd ~/migca
        $ sudo cp agent.crt /etc/mig/agent.crt
        $ sudo cp agent.key /etc/mig/agent.key
        $ sudo cp ca/ca.crt /etc/mig/ca.crt

Now edit the agent configuration file we installed, and modify the ``certs`` section
to reference our certificates and keys.

Next edit the agent configuration, and modify the ``relay`` parameter in the ``agent``
section to point to the URL of the RabbitMQ endpoint we setup. Note this parameter
also requires you include the agents RabbitMQ username and password. You will also
want to change the protocol from ``amqp`` to ``amqps``, and change the port to ``5671``.

Next, modify the ``api`` parameter under ``agent`` to point to the URL of the API
we configured earlier in this guide.

Proxy support
~~~~~~~~~~~~~

The agent supports connecting to the relay via a CONNECT proxy. If proxies are
configured, it will attempt to use them before attemping a direct connection. The
agent will also attempt to use any proxy noted in the environment via the
``HTTP_PROXY`` environment variable.

An agent using a proxy will reference the name of the proxy in the environment
fields of the heartbeat sent to the scheduler.

Stat socket
~~~~~~~~~~~

The agent can establish a listening TCP socket on localhost for management
purpose. You can browse to this socket (e.g., http://127.0.0.1:51664) to get
statistics from a running agent. The socket is also used internally by the agent
for various control messages. You will typically want to leave this value at it's
default setting.

Extra privacy mode (EPM)
~~~~~~~~~~~~~~~~~~~~~~~~

A design principle of MIG is that the agent protects privacy, and it will not
return information such as file contents or memory contents in any configuration.
It does however return meta-data that is useful to the investigator (such as
file names).

In some cases for example if you are running MIG on user workstations, you
may want to deploy extra privacy controls. Extra privacy mode informs the agent
that it should further mask certain result data. If enabled for example, the
file module will report that it found something as the result of a search, but
will not include file names.

It is up to modules to honor the EPM setting; currently this value is used by
the file module (mask filenames), the netstat module (mask addresses the system
is communicating with), and the scribe module (mask test identifiers).

EPM can be enabled using the ``extraprivacymode`` setting in the configuration file.

Logging
~~~~~~~

The agent can log to stdout, to a file or to the system logging. On Windows,
the system logging is the Event log. On POSIX systems, it's syslog.

Logging can be configured using the ``logging`` section in the configuration file,
by default the agent logs to stdout, which is suitable when running under a
supervisor process like systemd.

Access Control Lists
~~~~~~~~~~~~~~~~~~~~

At this point the agent can be run, but will not reply to actions sent to it
by an investigator as it does not have any knowledge of investigator public keys.
We need to configure ACLs and add the investigators keys to the keyring.

The details of how access control lists are created and managed is described in
`concepts: Access Control Lists`_. In this documentation, we focus on a basic
setup that grant access of all modules to all investigators, and restricts
what the scheduler key can do.

.. _`concepts: Access Control Lists`: concepts.rst

ACL are declared in JSON and are stored in ``/etc/mig/acl.cfg``. The agent
reads this file on startup to load it's ACL configuration. For now, we will
create two ACLs. A ``default`` ACL that grants access to all modules for two
investigators, and an ``agentdestroy`` ACL that grants access to the ``agentdestroy``
module to the scheduler.

The ACLs reference the fingerprint of the public key of each investigator
and a weight that describes how much permission each investigator is granted with.

.. code::

	{
		"default": {
			"minimumweight": 2,
			"investigators": {
				"Bob The Investigator": {
					"fingerprint": "E60892BB9BD89A69F759A1A0A3D652173B763E8F",
                                        "weight": 2
				},
				"Sam Axe": {
					"fingerprint": "FA5D79F95F7AF7097C3E83DA26A86D5E5885AC11",
					"weight": 2
				}
			}
		},
		"agentdestroy": {
			"minimumweight": 1,
			"investigators": {
				"MIG Scheduler": {
					"fingerprint": "A8E1ED58512FCD9876DBEA4FEA513B95032D9932",
					"weight": 1
				}
			}
		}
	}

Note that the PGP key of the scheduler was created automatically when we
started the scheduler service for the first time. You can access its
fingerprint via the mig-console, as follow:

.. code::

	$ mig-console
	mig> investigator 1
	inv 1> details
	Investigator ID 1
	name         migscheduler
	status       active
        permissions
	key id       A8E1ED58512FCD9876DBEA4FEA513B95032D9932
	created      2015-09-09 00:25:47.225086 -0400 EDT
	modified     2015-09-09 00:25:47.225086 -0400 EDT
        api key set  false

You can also view its public key by entering ``pubkey`` in the prompt.

Configure the agent keyring
~~~~~~~~~~~~~~~~~~~~~~~~~~~

The agent needs to be aware of the public keys associated with investigators
so it can verify the signatures on signed investigation actions it receives.
To add the keys to the agents keyring, create the directory to store them
and copy each ascii armored public key into this directory. Each key should be
in it's own file. The name of the files do not matter, so you can choose to
name them anything.

.. code:: bash

        $ sudo mkdir /etc/mig/agentkeys
        $ sudo cp mypubkey.txt /etc/mig/agentkeys/mypubkey
        $ sudo cp schedulerkey.txt /etc/mig/agentkeys/scheduler

Since all investigators must be created via the mig-console to have access
to the API, the easiest way to export their public keys is also via the mig-console.

.. code:: bash

	$ mig-console

	mig> investigator 2

	inv 2> pubkey
	-----BEGIN PGP PUBLIC KEY BLOCK-----
	Version: GnuPG v1

	mQENBFF/69EBCADe79sqUKJHXTMW3tahbXPdQAnpFWXChjI9tOGbgxmse1eEGjPZ
	QPFOPgu3O3iij6UOVh+LOkqccjJ8gZVLYMJzUQC+2RJ3jvXhti8xZ1hs2iEr65Rj
	zUklHVZguf2Zv2X9Er8rnlW5xzplsVXNWnVvMDXyzx0ufC00dDbCwahLQnv6Vqq8
	...

Customize the configuration
~~~~~~~~~~~~~~~~~~~~~~~~~~~

The agent has many other configuration parameters that you may want to
tweak before shipping it. Each of them is documented in the sample
configuration file.

Install the agent
~~~~~~~~~~~~~~~~~

With the agent configured, we will build an agent with the default modules
here and install it.

.. code:: bash

        $ make mig-agent
        $ sudo cp bin/linux/amd64/mig-agent-latest /opt/mig/bin/mig-agent

To cross-compile for a different platform, use the ``ARCH`` and ``OS`` make
variables:

.. code:: bash

	$ make mig-agent BUILDENV=prod OS=windows ARCH=amd64

We can test the agent on the command line using the debug flag ``-d``. When run
with ``-d``, the agent will stay in foreground and print its activity to stdout.

.. code:: bash

	$ sudo /opt/mig/bin/mig-agent -d
	[info] using builtin conf
	2015/09/09 10:43:30 - - - [debug] leaving initChannels()
	2015/09/09 10:43:30 - - - [debug] Logging routine initialized.
	2015/09/09 10:43:30 - - - [debug] leaving findHostname()
	2015/09/09 10:43:30 - - - [debug] Ident is Debian testing-updates sid
	2015/09/09 10:43:30 - - - [debug] Init is upstart
	2015/09/09 10:43:30 - - - [debug] leaving findOSInfo()
	2015/09/09 10:43:30 - - - [debug] Found local address 172.21.0.3/20
	2015/09/09 10:43:30 - - - [debug] Found local address fe80::3602:86ff:fe2b:6fdd/64
	2015/09/09 10:43:30 - - - [debug] Found public ip 172.21.0.3
	2015/09/09 10:43:30 - - - [debug] leaving initAgentID()
	2015/09/09 10:43:30 - - - [debug] Loading permission named 'default'
	2015/09/09 10:43:30 - - - [debug] Loading permission named 'agentdestroy'
	2015/09/09 10:43:30 - - - [debug] leaving initACL()
	2015/09/09 10:43:30 - - - [debug] AMQP: host=rabbitmq.mig.example.net, port=5671, vhost=mig
	2015/09/09 10:43:30 - - - [debug] Loading AMQPS TLS parameters
	2015/09/09 10:43:30 - - - [debug] Establishing connection to relay
	2015/09/09 10:43:30 - - - [debug] leaving initMQ()
	2015/09/09 10:43:30 - - - [debug] leaving initAgent()
	2015/09/09 10:43:30 - - - [info] Mozilla InvestiGator version 20150909+556e9c0.dev: started agent gator1
	2015/09/09 10:43:30 - - - [debug] heartbeat '{"name":"gator1","queueloc":"linux.gator1.ft8dzivx8zxd1mu966li7fy4jx0v999cgfap4mxhdgj1v0zv","mode":"daemon","version":"20150909+556e9c0.dev","pid":2993,"starttime":"2015-09-09T10:43:30.871448608-04:00","destructiontime":"0001-01-01T00:00:00Z","heartbeatts":"2015-09-09T10:43:30.871448821-04:00","environment":{"init":"upstart","ident":"Debian testing-updates sid","os":"linux","arch":"amd64","isproxied":false,"addresses":["172.21.0.3/20","fe80::3602:86ff:fe2b:6fdd/64"],"publicip":"172.21.0.3"},"tags":{"operator":"example.net"}}'
	2015/09/09 10:43:30 - - - [debug] Message published to exchange 'toschedulers' with routing key 'mig.agt.heartbeats' and body '{"name":"gator1","queueloc":"linux.gator1.ft8dzivx8zxd1mu966li7fy4jx0v999cgfap4mxhdgj1v0zv","mode":"daemon","version":"20150909+556e9c0.dev","pid":2993,"starttime":"2015-09-09T10:43:30.871448608-04:00","destructiontime":"0001-01-01T00:00:00Z","heartbeatts":"2015-09-09T10:43:30.871448821-04:00","environment":{"init":"upstart","ident":"Debian testing-updates sid","os":"linux","arch":"amd64","isproxied":false,"addresses":["172.21.0.3/20","fe80::3602:86ff:fe2b:6fdd/64"],"publicip":"172.21.0.3"},"tags":{"operator":"example.net"}}'
	2015/09/09 10:43:30 - - - [debug] leaving initSocket()
	2015/09/09 10:43:30 - - - [debug] leaving publish()
	2015/09/09 10:43:30 - - - [info] Stat socket connected successfully on 127.0.0.1:61664
	^C2015/09/09 10:43:39 - - - [emergency] Shutting down agent: 'interrupt'
	2015/09/09 10:43:40 - - - [info] closing sendResults channel
	2015/09/09 10:43:40 - - - [info] closing parseCommands goroutine
	2015/09/09 10:43:40 - - - [info] closing runModule goroutine

The output above indicates that the agent successfully connected to RabbitMQ
and sent a heartbeat message. The scheduler will receive this heartbeat and
process it, indicating to the scheduler the agent is online.

At the next run of the scheduler periodic routine, the agent will be marked
as ``online`` and show up in the dashboard counters. You can browse these counters
using the ``mig-console``.

.. code::

	mig> status
	+------
	| Agents & Endpoints summary:
	| * 1 online agents on 1 endpoints
	+------

Now that we have confirmed the agent works as expected, run the agent normally without
the debug flag.

.. code:: bash

        $ sudo /opt/mig/bin/mig-agent

This will cause the agent to identify the init system in use, and install itself as a service
and subsequently start itself up in daemon mode.

Run your first investigation
----------------------------

We will run an investigation using the ``mig`` command, which is different from ``mig-console``
in that it is more intended for quicker simplified investigations. We can install
the ``mig`` command and run a simple investigation that looks for a user in ``/etc/passwd``.

.. code:: bash

        $ make mig-cmd
        $ sudo cp bin/linux/amd64/mig /opt/mig/bin/mig
	$ /opt/mig/bin/mig file -t allonline -path /etc -name "^passwd$" -content "^root"
	1 agents will be targeted. ctrl+c to cancel. launching in 5 4 3 2 1 GO
	Following action ID 4885615083564.status=inflight.
	- 100.0% done in -2m17.141481302s
	1 sent, 1 done, 1 succeeded
	gator1 /etc/passwd [lastmodified:2015-08-31 16:15:05.547605529 +0000 UTC, mode:-rw-r--r--, size:2251] in search 's1'
	1 agent has found results

A single file is found, as expected.

Appendix A: Advanced RabbitMQ Configuration
-------------------------------------------

RabbitMQ can be configured in a variety of ways, and this guide does not discuss
RabbitMQ configuration in detail. For details on RabbitMQ consult the
RabbitMQ documentation at https://www.rabbitmq.com/documentation.html. A couple
points for consideration are noted in this section however.

Queue mirroring
~~~~~~~~~~~~~~~

By default, queues within a RabbitMQ cluster are located on a single node (the
node on which they were first declared). If that node goes down, the queue will
become unavailable. To mirror all MIG queues to all nodes of a rabbitmq cluster,
use the following policy:

.. code:: bash

	# rabbitmqctl -p mig set_policy mig-mirror-all "^mig\." '{"ha-mode":"all"}'
	Setting policy "mig-mirror-all" for pattern "^mig\\." to "{\"ha-mode\":\"all\"}" with priority "0" ...
	...done.

Cluster management
~~~~~~~~~~~~~~~~~~

To create a cluster, all RabbitMQ nodes must share a secret called erlang
cookie. The erlang cookie is located in ``/var/lib/rabbitmq/.erlang.cookie``.
Make sure the value of the cookie is identical on all members of the cluster,
then tell one node to join another one:

.. code:: bash

	# rabbitmqctl stop_app
	Stopping node 'rabbit@ip-172-30-200-73' ...
	...done.

	# rabbitmqctl join_cluster rabbit@ip-172-30-200-42
	Clustering node 'rabbit@ip-172-30-200-73' with 'rabbit@ip-172-30-200-42' ...
	...done.

	# rabbitmqctl start_app
	Starting node 'rabbit@ip-172-30-200-73' ...
	...done.

To remove a dead node from the cluster, use the following command from any
active node of the running cluster.

.. code:: bash

	# rabbitmqctl forget_cluster_node rabbit@ip-172-30-200-84

If one node of the cluster goes down, and the agents have trouble reconnecting,
they may throw the error `NOT_FOUND - no binding mig.agt....`. That happens when
the binding in question exists but the 'home' node of the (durable) queue is not
alive. In case of a mirrored queue that would imply that all mirrors are down.
Essentially both the queue and associated bindings are in a limbo state at that
point - they neither exist nor do they not exist. `source`_

.. _`source`: http://rabbitmq.1065348.n5.nabble.com/Can-t-Bind-After-Upgrading-from-3-1-1-to-3-1-5-td29793.html

The safest thing to do is to delete all the queues on the cluster, and restart
the scheduler. The agents will restart themselves.

.. code:: bash

	# for queue in $(rabbitmqctl list_queues -p mig|grep ^mig|awk '{print $1}')
	do
		echo curl -i -u admin:adminpassword -H "content-type:application/json" \
		-XDELETE http://localhost:15672/api/queues/mig/$queue;
	done

(remove the ``echo`` in the command above, it's there as a safety for copy/paste
people).

Supporting more than 1024 connections
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

If you want more than 1024 clients, you may have to increase the max number of
file descriptors that RabbitMQ is allowed to hold. On Linux, increase ``nofile``
in ``/etc/security/limits.conf`` as follow:

.. code:: bash

	rabbitmq - nofile 102400

Then, make sure that ``pam_limits.so`` is included in ``/etc/pam.d/common-session``:

.. code:: bash

	session    required     pam_limits.so

This is an example, and configuration of this parameter may be different for your
environment.

Serving AMQPS on port 443
~~~~~~~~~~~~~~~~~~~~~~~~~

To prevent yours agents from getting blocked by firewalls, it may be a good idea
to use port 443 for connections between Agents and RabbitMQ. However, RabbitMQ
is not designed to run on a privileged port. The solution, then, is to use
iptables to redirect the port on the RabbitMQ server.

.. code:: bash

	# iptables -t nat -A PREROUTING -i eth0 -p tcp --dport 443 -j REDIRECT --to-port 5671 -m comment --comment "Serve RabbitMQ on HTTPS port"

You can also use something like an AWS ELB in TCP mode to provide access to your relay
on port 443.

Appendix B: Scheduler configuration reference
---------------------------------------------

Spool directories
~~~~~~~~~~~~~~~~~

The scheduler keeps copies of work in progress in a set of spool directories.
It will take care of creating the spool if it doesn't exist. The spool shouldn't grow
in size beyond a few megabytes as the scheduler tries to do regular housekeeping,
but it is still preferable to put it in a large enough location.

The standard location for this is ``/var/cache/mig``.

Database tuning
~~~~~~~~~~~~~~~

**sslmode**

``sslmode`` can take the values ``disable`, ``require`` (no cert verification)
and ``verify-full`` (requires cert verification). A proper installation should
use ``verify-full``.

.. code::

	[postgres]
		sslmode = "verify-full"

**maxconn**

The scheduler has an extra parameter to control the max number of database
connections it can use at once. It's important to keep that number relatively
low, and increase it with the size of your infrastructure. The default value is
set to ``10``.

.. code::

	[postgres]
		maxconn = 10

If the DB insertion rate is lower than the agent heartbeats rate, the scheduler
will receive more heartbeats per seconds than it can insert in the database.
When that happens, you will see the insertion lag increase in the query below:

.. code:: sql

	mig=> select NOW() - heartbeattime as "insertion lag"
	mig-> from agents order by heartbeattime desc limit 1;
	  insertion lag
	-----------------
	 00:00:00.212257
	(1 row)

A healthy insertion lag should be below one second. If the lag increases, and
your database server still isn't stuck at 100% CPU, try increasing the value of
``maxconn``. It will cause the scheduler to use more insertion threads.
