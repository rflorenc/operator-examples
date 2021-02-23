package constants

const (
	ControllerEventSourceComponent = "secret-watcher"
	SecretPrefix                   = "secret#"
	HashAnnotation                 = "watcher.k8s.io/data-hash"
	WatcherSecretsAnnotation       = "watcher.k8s.io/watch-secrets"
)
