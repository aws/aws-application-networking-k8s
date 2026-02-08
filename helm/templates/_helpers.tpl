{{/* The name of the application this chart installs */}}
{{- define "app.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "app.fullname" -}}
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

{{/* The name and version as used by the chart label */}}
{{- define "chart.name-version" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* The name of the service account to use */}}
{{- define "service-account.name" -}}
    {{ default "default" .Values.serviceAccount.name }}
{{- end -}}

{{/* Import or generate certificates for webhook */}}
{{- define "aws-gateway-controller.webhookTLS" -}}
{{- if (and .Values.webhookTLS.caCert .Values.webhookTLS.cert .Values.webhookTLS.key) -}}
caCert: {{ .Values.webhookTLS.caCert }}
cert: {{ .Values.webhookTLS.cert }}
key: {{ .Values.webhookTLS.key }}
{{- else -}}
{{- $ca := genCA "aws-gateway-controller-ca" 365 -}}
{{- $serviceDefaultName:= printf "webhook-service.%s.svc" .Release.Namespace -}}
{{- $secretName := "webhook-cert" -}}
{{- $altNames := list ($serviceDefaultName) (printf "%s.cluster.local" $serviceDefaultName) -}}
{{- $cert := genSignedCert $serviceDefaultName nil $altNames 365 $ca -}}
caCert: {{ $ca.Cert | b64enc }}
cert: {{ $cert.Cert | b64enc }}
key: {{ $cert.Key | b64enc }}
{{- end -}}
{{- end -}}
