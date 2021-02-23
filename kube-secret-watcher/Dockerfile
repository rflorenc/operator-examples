FROM openshift/origin-release:golang-1.9

RUN GOBIN=/usr/bin go get github.com/rflorenc/kube-secret-watcher

CMD ["/usr/bin/kube-secret-watcher", "-v", "8", "logtostderr", "-alsologtostderr"]
