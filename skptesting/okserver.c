/*
okserver is a minimal HTTP server that responds to every request with
a fixed 200 response. It is used by the load balancer benchmarks to
run a large number of backend processes with a small footprint.

Build:

	cc -O2 -o okserver okserver.c -lpthread

Usage:

	okserver <port> [count]

When count is given, the server listens on all ports from port to
port+count-1 in a single process. This allows simulating many endpoints
on systems where the process limit does not allow one process per
endpoint.
*/
#include <arpa/inet.h>
#include <netinet/in.h>
#include <netinet/tcp.h>
#include <pthread.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <unistd.h>

static const char response[] =
    "HTTP/1.1 200 OK\r\n"
    "Content-Type: text/plain\r\n"
    "Content-Length: 3\r\n"
    "\r\n"
    "OK\n";

static void *serve(void *arg) {
	int conn = (int)(long)arg;
	char buf[4096];

	for (;;) {
		/* read one request, assume no body and that the header
		   arrives in one segment, which holds for benchmark
		   clients on loopback */
		ssize_t n = read(conn, buf, sizeof(buf));
		if (n <= 0)
			break;

		if (write(conn, response, sizeof(response)-1) < 0)
			break;
	}

	close(conn);
	return NULL;
}

static void *accept_loop(void *arg) {
	int listener = (int)(long)arg;
	int one = 1;

	for (;;) {
		int conn = accept(listener, NULL, NULL);
		if (conn < 0)
			continue;

		setsockopt(conn, IPPROTO_TCP, TCP_NODELAY, &one, sizeof(one));

		pthread_t t;
		if (pthread_create(&t, NULL, serve, (void *)(long)conn) == 0)
			pthread_detach(t);
		else
			close(conn);
	}

	return NULL;
}

static int listen_on(int port) {
	int listener = socket(AF_INET, SOCK_STREAM, 0);
	if (listener < 0) {
		perror("socket");
		return -1;
	}

	int one = 1;
	setsockopt(listener, SOL_SOCKET, SO_REUSEADDR, &one, sizeof(one));

	struct sockaddr_in addr;
	memset(&addr, 0, sizeof(addr));
	addr.sin_family = AF_INET;
	addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
	addr.sin_port = htons((unsigned short)port);

	if (bind(listener, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
		perror("bind");
		return -1;
	}

	if (listen(listener, 1024) < 0) {
		perror("listen");
		return -1;
	}

	return listener;
}

int main(int argc, char **argv) {
	if (argc != 2 && argc != 3) {
		fprintf(stderr, "usage: %s <port> [count]\n", argv[0]);
		return 1;
	}

	signal(SIGPIPE, SIG_IGN);

	int port = atoi(argv[1]);
	int count = argc == 3 ? atoi(argv[2]) : 1;
	if (count < 1)
		count = 1;

	for (int i = 0; i < count; i++) {
		int listener = listen_on(port + i);
		if (listener < 0)
			return 1;

		if (i == count-1) {
			/* run the last accept loop on the main thread */
			accept_loop((void *)(long)listener);
			return 0;
		}

		pthread_t t;
		if (pthread_create(&t, NULL, accept_loop, (void *)(long)listener) != 0) {
			perror("pthread_create");
			return 1;
		}
		pthread_detach(t);
	}

	return 0;
}
