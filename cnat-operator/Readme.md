## cnat-operator example
Updated build with operator-sdk v0.19.0

## Dev environment setup
`$ kind --version`
```
kind version 0.7.0
```

`$ operator-sdk version`
```
operator-sdk version: "v0.19.0", commit: "8e28aca60994c5cb1aec0251b85f0116cc4c9427", kubernetes version: "v1.18.2", go version: "go1.14.4 darwin/amd64"
```

`kubectl apply -f config/crd/bases/cn.example.com_ats.yaml`

`$ make install && make run`

Under config/samples/cn_v1alpha1_at.yaml, adapt spec.schedule and spec.command accordingly.

`$ kubectl apply -f config/samples/cn_v1alpha1_at.yaml`

# Step by step
`$ kind create cluster`

`$ mkdir $(go env GOPATH)/src/github.com/rflorenc/operator-examples/cnat-operator`

`$ cd $(go env GOPATH)/src/github.com/rflorenc/operator-examples/cnat-operator`

`$ operator-sdk init --repo github.com/rflorenc/operator-examples/cnat-operator --domain example.com`

`$ operator-sdk create api --group cn --version=v1alpha1 --kind At`

```
Create Resource [y/n]
y
Create Controller [y/n]
y
```

`$ Modify at_types.go and at_controller.go accordingly.` 

`$ kubectl apply -f config/crd/bases/cn.example.com_ats.yaml`

`$ make install && make run`

`$ kubectl apply -f config/samples/cn_v1alpha1_at.yaml`

Check operator logs
```
time.Now() is: 2020-07-28 14:49:11.003043 +0200 CEST m=+288.178663244
Time until schedule: -3.043ms2020-07-28T14:49:11.003+0200       INFO    controllers.At  Ready to execute        {"At": "cnat/at-sample"}
2020-07-28T14:49:11.026+0200    DEBUG   controller-runtime.controller   Successfully Reconciled {"controller": "at", "request": "cnat/at-sample"}
2020-07-28T14:49:11.026+0200    INFO    controllers.At  Phase: RUNNING  {"At": "cnat/at-sample"}
2020-07-28T14:49:11.151+0200    INFO    controllers.At  Pod launched    {"At": "cnat/at-sample", "name": "at-sample-pod"}
2020-07-28T14:49:11.181+0200    DEBUG   controller-runtime.controller   Successfully Reconciled {"controller": "at", "request": "cnat/at-sample"}
```

`$ kubectl get pods -n cnat`
```
NAME            READY   STATUS      RESTARTS   AGE
at-sample-pod   0/1     Completed   0          2m11s
```

