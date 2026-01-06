/*
Package swarm implements the exchange of information between
Skipper instances using a gossip protocol called SWIM.  This
aims at a solution that can work in a context of multiple readers
and writers, with the guarantee of low latency, weakly consistent
data, from which derives the decision to use such protocol. As an
example the implementation of the filter clusterRatelimit uses
the swarm data exchange to have a global state of current requests.

A swarm instance needs to find some of its peers before joining the
cluster. Current implementations to find peers are swarmKubernetes to
find skipper instances running in a Kubernetes cluster and swarmFake
for testing.

Background information:

The current skipper implementation uses
hashicorp's memberlist, https://github.com/hashicorp/memberlist,
which is an implementation of the swim protocol. You can find a
detailed paper at
http://www.cs.cornell.edu/~asdas/research/dsn02-SWIM.pdf.

Quote from a nice overview https://prakhar.me/articles/swim/

	The SWIM or the Scalable Weakly-consistent Infection-style process
	group Membership protocol is a protocol used for maintaining
	membership amongst processes in a distributed system.

While starting, Skipper will find its swarm peers through the
Kubernetes API server. It will do that using a label selector query
to find Pods of the swarm.
*/
package swarm
