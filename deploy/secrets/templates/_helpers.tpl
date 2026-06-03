{{/*
Возвращает значение пароля/ключа: из существующего Secret, из legacy Secret чарта,
или генерирует новый (только при первом install).
*/}}
{{- define "secrets.value" -}}
{{- $ns := .root.Release.Namespace -}}
{{- $secretName := .name -}}
{{- $key := .key -}}
{{- $legacyName := .legacyName | default "" -}}
{{- $legacyKey := .legacyKey | default "" -}}
{{- $gen := .gen | default "alpha" -}}
{{- $existing := lookup "v1" "Secret" $ns $secretName -}}
{{- if and $existing $existing.data (index $existing.data $key) -}}
{{- index $existing.data $key | b64dec -}}
{{- else if and $legacyName $legacyKey -}}
{{- $legacy := lookup "v1" "Secret" $ns $legacyName -}}
{{- if and $legacy $legacy.data (index $legacy.data $legacyKey) -}}
{{- index $legacy.data $legacyKey | b64dec -}}
{{- else if eq $gen "hex" -}}
{{- randAlphaNum 32 -}}
{{- else -}}
{{- randAlphaNum 24 -}}
{{- end -}}
{{- else if eq $gen "hex" -}}
{{- randAlphaNum 32 -}}
{{- else -}}
{{- randAlphaNum 24 -}}
{{- end -}}
{{- end -}}
