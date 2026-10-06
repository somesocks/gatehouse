{{- define "gatehouse.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "gatehouse.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "gatehouse.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gatehouse.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "gatehouse.podLabels" -}}
{{- $labels := deepCopy .Values.podLabels -}}
{{- $_ := set $labels "app.kubernetes.io/name" (include "gatehouse.name" .) -}}
{{- $_ := set $labels "app.kubernetes.io/instance" .Release.Name -}}
{{- $_ := set $labels "app.kubernetes.io/component" "server" -}}
{{- toYaml $labels -}}
{{- end -}}

{{- define "gatehouse.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "gatehouse.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "gatehouse.volumeSource" -}}
{{- $root := .root -}}
{{- $volume := .volume -}}
{{- $source := deepCopy $volume.source -}}
{{- $pvc := get $source "persistentVolumeClaim" -}}
{{- if and $pvc (hasKey $pvc "create") -}}
{{- $claim := dict "claimName" (printf "%s-%s" (include "gatehouse.fullname" $root) $volume.name | trunc 63 | trimSuffix "-") -}}
{{- if hasKey $pvc "readOnly" -}}
{{- $_ := set $claim "readOnly" (get $pvc "readOnly") -}}
{{- end -}}
{{- $_ := set $source "persistentVolumeClaim" $claim -}}
{{- end -}}
{{- toYaml $source -}}
{{- end -}}
