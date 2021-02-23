package util

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

func lastHash(kind, name string) string {
	return fmt.Sprintf("watcher.k8s.io/%s-%s-last-hash", kind, name)
}

// GetSHA1hash generates a SHA1 hash from the map[string]strings
func GetSHA1hash(obj interface{}) string {
	sha1Hash := sha1.New()
	switch t := obj.(type) {
	case *corev1.Secret:
		var (
			sortedDataKeys       []string
			sortedStringDataKeys []string
		)
		for k := range t.Data {
			sortedDataKeys = append(sortedDataKeys, k)
		}
		for k := range t.StringData {
			sortedStringDataKeys = append(sortedStringDataKeys, k)
		}
		if len(sortedDataKeys) == 0 && len(sortedStringDataKeys) == 0 {
			return ""
		}
		sort.Strings(sortedDataKeys)
		sha1Hash.Write([]byte(strings.Join(sortedDataKeys, "")))
		sort.Strings(sortedStringDataKeys)
		sha1Hash.Write([]byte(strings.Join(sortedStringDataKeys, "")))

		for _, key := range sortedDataKeys {
			sha1Hash.Write(t.Data[key])
		}
		for _, key := range sortedStringDataKeys {
			sha1Hash.Write([]byte(t.StringData[key]))
		}
	default:
		utilruntime.HandleError(fmt.Errorf("Unknown object: %v", obj))
		return ""
	}
	return hex.EncodeToString(sha1Hash.Sum(nil))
}
