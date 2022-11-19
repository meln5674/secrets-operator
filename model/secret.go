package model

import (
	"encoding/base64"
	"fmt"
	sprig "github.com/Masterminds/sprig/v3"
	secretsv1alpha1 "github.com/meln5674/secrets-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
	"strings"
	templates "text/template"
)

const (
	DefaultSecretType = corev1.SecretTypeOpaque
)

var (
	CustomFuncs = map[string]interface{}{
		"b64bin": base64.StdEncoding.EncodeToString,
		"utf8":   func(x []byte) string { return string(x) },
	}
)

type TemplateContext struct {
	References map[string]map[string]interface{}
}

func GenerateSecret(cmRefs map[string]corev1.ConfigMap, sRefs map[string]corev1.Secret, src *secretsv1alpha1.DerivedSecret) (secret corev1.Secret, noOverwrite map[string]struct{}, err error) {
	noOverwrite = make(map[string]struct{})

	for key, tgt := range src.Spec.Data {
		overwrite := secretsv1alpha1.DefaultTargetOverwrite
		if tgt.Overwrite != nil {
			overwrite = *tgt.Overwrite
		}
		if !overwrite {
			noOverwrite[key] = struct{}{}
		}
	}
	for key, tgt := range src.Spec.StringData {
		overwrite := secretsv1alpha1.DefaultTargetOverwrite
		if tgt.Overwrite != nil {
			overwrite = *tgt.Overwrite
		}
		if !overwrite {
			noOverwrite[key] = struct{}{}
		}
	}

	blank := corev1.Secret{}
	target := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      strOrDefault(src.Spec.TargetName, src.Name),
			Namespace: strOrDefault(src.Spec.TargetNamespace, src.Namespace),
			Labels:    secretsv1alpha1.DerivedFromLabelValues(src),
		},
		Type:       corev1.SecretType(strOrDefault(string(src.Spec.TargetType), string(DefaultSecretType))),
		Data:       make(map[string][]byte),
		StringData: make(map[string]string),
	}

	references := make(map[string]map[string]interface{})
	for ref, cm := range cmRefs {
		references[ref] = make(map[string]interface{})
		for key, value := range cm.Data {
			references[ref][key] = value
		}
		for key, value := range cm.BinaryData {
			references[ref][key] = value
		}
	}
	for ref, s := range sRefs {
		references[ref] = make(map[string]interface{})
		for key, value := range s.Data {
			references[ref][key] = value
		}
		for key, value := range s.StringData {
			references[ref][key] = value
		}
	}
	if src.Spec.Prefab != nil && src.Spec.Prefab.CopyAll != nil && *src.Spec.Prefab.CopyAll {
		knownKeys := make(map[string]string)
		for ref, cm := range cmRefs {
			collision, collidingKey, collided := copyMapPair(ref, knownKeys, target.StringData, cm.Data, target.Data, cm.BinaryData)
			if collided {
				return blank, nil, fmt.Errorf("Key %s in reference %s was also present in reference %s when doing a prefab.copyAll", collidingKey, ref, collision)
			}
		}
		for ref, s := range sRefs {
			collision, collidingKey, collided := copyMapPair(ref, knownKeys, target.StringData, s.StringData, target.Data, s.Data)
			if collided {
				return blank, nil, fmt.Errorf("Key %s in reference %s was also present in reference %s when doing a prefab.copyAll", collidingKey, ref, collision)
			}
		}
		return target, noOverwrite, nil
	}
	if src.Spec.Prefab != nil && len(src.Spec.Prefab.CopyIncluding) != 0 {
		included := make(map[string]map[string]struct{})
		for _, include := range src.Spec.Prefab.CopyIncluding {
			included[include.Name] = make(map[string]struct{})
			ref, ok := references[include.Name]
			if !ok {
				return blank, nil, fmt.Errorf("prefab.copyInclude reference %s does not exist", include.Name)
			}
			if include.AllKeys != nil && *include.AllKeys {
				for key, _ := range ref {
					included[include.Name][key] = struct{}{}
				}
			} else {
				for _, key := range include.Keys {
					included[include.Name][key] = struct{}{}
				}
			}
		}

		knownKeys := make(map[string]string)
		for ref, cm := range cmRefs {
			collision, collidingKey, collided := copyMapPairInclude(ref, knownKeys, included[ref], target.StringData, cm.Data, target.Data, cm.BinaryData)
			if collided {
				return blank, nil, fmt.Errorf("Key %s in reference %s was also present in reference %s when doing a prefab.copyIncluding", collidingKey, ref, collision)
			}
		}
		for ref, s := range sRefs {
			collision, collidingKey, collided := copyMapPairInclude(ref, knownKeys, included[ref], target.StringData, s.StringData, target.Data, s.Data)
			if collided {
				return blank, nil, fmt.Errorf("Key %s in reference %s was also present in reference %s when doing a prefab.copyIncluding", collidingKey, ref, collision)
			}
		}
		return target, noOverwrite, nil

	}
	if src.Spec.Prefab != nil && len(src.Spec.Prefab.CopyExcluding) != 0 {
		excluded := make(map[string]map[string]struct{})
		for _, exclude := range src.Spec.Prefab.CopyExcluding {
			excluded[exclude.Name] = make(map[string]struct{})
			ref, ok := references[exclude.Name]
			if !ok {
				return blank, nil, fmt.Errorf("prefab.copyExclude reference %s does not exist", exclude.Name)
			}
			if exclude.AllKeys != nil && *exclude.AllKeys {
				for key, _ := range ref {
					excluded[exclude.Name][key] = struct{}{}
				}
			} else {
				for _, key := range exclude.Keys {
					excluded[exclude.Name][key] = struct{}{}
				}
			}
		}

		knownKeys := make(map[string]string)
		for ref, cm := range cmRefs {
			collision, collidingKey, collided := copyMapPairExclude(ref, knownKeys, excluded[ref], target.StringData, cm.Data, target.Data, cm.BinaryData)
			if collided {
				return blank, nil, fmt.Errorf("Key %s in reference %s was also present in reference %s when doing a prefab.copyExcluding", collidingKey, ref, collision)
			}
		}
		for ref, s := range sRefs {
			collision, collidingKey, collided := copyMapPairExclude(ref, knownKeys, excluded[ref], target.StringData, s.StringData, target.Data, s.Data)
			if collided {
				return blank, nil, fmt.Errorf("Key %s in reference %s was also present in reference %s when doing a prefab.copyExcluding", collidingKey, ref, collision)
			}
		}
		return target, noOverwrite, nil

	}

	knownKeys := make(map[string]struct{})
	context := TemplateContext{References: references}
	for key, tgt := range src.Spec.Data {
		if _, collided := knownKeys[key]; collided {
			return blank, nil, fmt.Errorf("Key %s appeared in multiple locations between data, stringData, and the output of map templates", key)
		}
		knownKeys[key] = struct{}{}

		if tgt.Literal != nil {
			target.Data[key] = tgt.Literal
			continue
		}
		var template string
		if tgt.Template != nil {
			template = *tgt.Template
		}

		tpl, err := templates.New(key).Funcs(sprig.TxtFuncMap()).Funcs(CustomFuncs).Parse(template)
		if err != nil {
			return blank, nil, fmt.Errorf(`Failed to parse spec.data["%s"] as a template: %s`, key, err)
		}

		out := strings.Builder{}
		if err = tpl.Execute(&out, &context); err != nil {
			return blank, nil, err
		}
		isMap := secretsv1alpha1.DefaultIsMap
		if tgt.IsMap != nil {
			isMap = *tgt.IsMap
		}
		if isMap {
			delete(knownKeys, key)
			mapData := make(map[string][]byte)
			err := yaml.Unmarshal([]byte(out.String()), &mapData)
			if err != nil {
				return blank, nil, fmt.Errorf(`Failed to parse output of spec.data["%s"] as yaml map of string to base64: %s`, key, err)
			}
			for key, value := range mapData {
				if _, collided := knownKeys[key]; collided {
					return blank, nil, fmt.Errorf("Key %s appeared in multiple locations between data, stringData, and the output of map templates", key)
				}
				knownKeys[key] = struct{}{}
				target.Data[key] = value
			}
		} else {
			target.Data[key], err = base64.StdEncoding.DecodeString(out.String())
			if err != nil {
				return blank, nil, fmt.Errorf(`Failed to decode spec.data["%s"] output as base64: %s`, key, err)
			}
		}

	}
	for key, tgt := range src.Spec.StringData {
		if _, collided := knownKeys[key]; collided {
			return blank, nil, fmt.Errorf("Key %s appeared in multiple locations between data, stringData, and the output of map templates", key)
		}
		knownKeys[key] = struct{}{}

		if tgt.Literal != nil {
			target.StringData[key] = *tgt.Literal
			continue
		}
		var template string
		if tgt.Template != nil {
			template = *tgt.Template
		}

		knownKeys[key] = struct{}{}
		tpl, err := templates.New(key).Funcs(sprig.TxtFuncMap()).Funcs(CustomFuncs).Parse(template)
		if err != nil {
			return blank, nil, fmt.Errorf(`Failed to parse spec.stringData["%s"] as a template: %s`, key, err)
		}

		out := strings.Builder{}
		if err = tpl.Execute(&out, &context); err != nil {
			return blank, nil, err
		}
		isMap := secretsv1alpha1.DefaultIsMap
		if tgt.IsMap != nil {
			isMap = *tgt.IsMap
		}
		if isMap {
			delete(knownKeys, key)
			mapData := make(map[string]string)
			err := yaml.Unmarshal([]byte(out.String()), &mapData)
			if err != nil {
				return blank, nil, fmt.Errorf(`Failed to parse output of spec.stringData["%s"] as yaml map of string to string: %s`, key, err)
			}
			for key, value := range mapData {
				if _, collided := knownKeys[key]; collided {
					return blank, nil, fmt.Errorf("Key %s appeared in multiple locations between data, stringData, and the output of map templates", key)
				}
				knownKeys[key] = struct{}{}
				target.StringData[key] = value
			}
		} else {
			target.StringData[key] = out.String()
		}
	}
	return target, noOverwrite, nil
}
