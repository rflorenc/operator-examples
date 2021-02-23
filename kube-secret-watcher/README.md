# kube-secret-watcher

This sample kubernetes controller mainly does two things:

* watches for changes to Secret resources via configurable annotation

* reacts to any change in Secret by creating a new Deployment


## Build and run

$ go build -o kube-secret-watcher

$ ./kube-secret-watcher -kubeConfig ~/.kube/config -v 8 -logtostderr

