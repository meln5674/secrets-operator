package model

func strOrDefault(a string, b string) string {
	if a == "" {
		return b
	}
	return a
}

func copyStrMapGeneric(knownId string, knownKeys map[string]string, include, exclude map[string]struct{}, target, src map[string]string) (string, string, bool) {
	for key, value := range src {
		if collision, collided := knownKeys[key]; collided {
			return collision, key, true
		}
		if include != nil {
			if _, included := include[key]; !included {
				continue
			}
		}
		if exclude != nil {
			if _, excluded := exclude[key]; excluded {
				continue
			}
		}
		knownKeys[key] = knownId
		target[key] = value
	}
	return "", "", false
}

func copyStrMap(knownId string, knownKeys map[string]string, target, src map[string]string) (string, string, bool) {
	return copyStrMapGeneric(knownId, knownKeys, nil, nil, target, src)
}

func copyStrMapInclude(knownId string, knownKeys map[string]string, include map[string]struct{}, target, src map[string]string) (string, string, bool) {
	return copyStrMapGeneric(knownId, knownKeys, include, nil, target, src)
}

func copyStrMapExclude(knownId string, knownKeys map[string]string, exclude map[string]struct{}, target, src map[string]string) (string, string, bool) {
	return copyStrMapGeneric(knownId, knownKeys, nil, exclude, target, src)
}

func copyBinMapGeneric(knownId string, knownKeys map[string]string, include, exclude map[string]struct{}, target, src map[string][]byte) (string, string, bool) {
	for key, value := range src {
		if collision, collided := knownKeys[key]; collided {
			return collision, key, true
		}
		if include != nil {
			if _, included := include[key]; !included {
				continue
			}
		}
		if exclude != nil {
			if _, excluded := exclude[key]; excluded {
				continue
			}
		}
		knownKeys[key] = knownId
		target[key] = value
	}
	return "", "", false
}

func copyBinMap(knownId string, knownKeys map[string]string, target, src map[string][]byte) (string, string, bool) {
	return copyBinMapGeneric(knownId, knownKeys, nil, nil, target, src)
}

func copyBinMapInclude(knownId string, knownKeys map[string]string, include map[string]struct{}, target, src map[string][]byte) (string, string, bool) {
	return copyBinMapGeneric(knownId, knownKeys, include, nil, target, src)
}

func copyBinMapExclude(knownId string, knownKeys map[string]string, exclude map[string]struct{}, target, src map[string][]byte) (string, string, bool) {
	return copyBinMapGeneric(knownId, knownKeys, nil, exclude, target, src)
}

func copyMapPairGeneric(knownId string, knownKeys map[string]string, include, exclude map[string]struct{}, target, src map[string]string, targetBin, srcBin map[string][]byte) (string, string, bool) {
	collision, collidingKey, collided := copyStrMapGeneric(knownId, knownKeys, include, exclude, target, src)
	if collided {
		return collision, collidingKey, collided
	}
	return copyBinMapGeneric(knownId, knownKeys, include, exclude, targetBin, srcBin)
}

func copyMapPair(knownId string, knownKeys map[string]string, target, src map[string]string, targetBin, srcBin map[string][]byte) (string, string, bool) {
	return copyMapPairGeneric(knownId, knownKeys, nil, nil, target, src, targetBin, srcBin)
}

func copyMapPairInclude(knownId string, knownKeys map[string]string, include map[string]struct{}, target, src map[string]string, targetBin, srcBin map[string][]byte) (string, string, bool) {
	return copyMapPairGeneric(knownId, knownKeys, include, nil, target, src, targetBin, srcBin)
}

func copyMapPairExclude(knownId string, knownKeys map[string]string, exclude map[string]struct{}, target, src map[string]string, targetBin, srcBin map[string][]byte) (string, string, bool) {
	return copyMapPairGeneric(knownId, knownKeys, nil, exclude, target, src, targetBin, srcBin)
}
