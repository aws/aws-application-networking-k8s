# HTTPS and Bring Your Own Certificte (BYOC) 
## Securing Traffic using HTTPS

Today, the HTTPRoute owner can specify all incoming traffic `MUST` use HTTPs.  e.g.

```
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: Gateway
metadata:
  name: my-hotel
spec:
  gatewayClassName: amazon-vpc-lattice
  listeners:
  - name: http
    protocol: HTTP
    port: 80
  - name: https <-------------- specify HTTPs listener
    protocol: HTTPS
    port: 443
```    

```
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: HTTPRoute
metadata:
  name: rates
spec:
  parentRefs:
  - name: my-hotel
    sectionName: http 
  - name: my-hotel
    sectionName: https  <--- specify all traffic MUST use HTTPs
  rules:
  - backendRefs:  
    - name: parking
      kind: Service
      port: 8090
    matches:
    - path:
        type: PathPrefix
        value: /parking
  - backendRefs:
    - name: review
      kind: Service
      port: 8090
    matches:
    - path:
        type: PathPrefix
        value: /review
```

In this case, VPC Lattice service will automatically generate a managed ACM certificate and use it for encryting client to service traffic.

## Bring Your Own Certificate (BYOC)

If customer desires to use custom domain name along with their own certificate, they can do following:
* follow [TODO Bring Your Own Certicate DOC](http://dev-dsk-tnmat-1d-8836d755.us-east-1.amazon.com/mercury/build/AWSMercuryDocs/AWSMercuryDocs-3.0/AL2_x86_64/DEV.STD.PTHREAD/build/server-root/vpc-lattice/latest/ug/service-byoc.html), and get  ACM certificate ARN
* specify certificate ARN 

```
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: Gateway
metadata:
  name: my-hotel
spec:
  gatewayClassName: amazon-vpc-lattice
  listeners:
  - name: http
    protocol: HTTP
    port: 80
  - name: https
    protocol: HTTPS
    port: 443
  - name: rates-with-custom-cert
    protocol: HTTPS
    port: 443
    tls:
      mode: Terminate
      options:
        application-networking.k8s.aws/certificate-arn: arn:aws:acm:us-west-2:<account>:certificate/4555204d-07e1-43f0-a533-d02750f41545 
```

* associate HTTPRoute to this

```
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: HTTPRoute
metadata:
  name: rates
spec:
  parentRefs:
  - name: my-hotel
    sectionName: http 
  - name: my-hotel
    sectionName: rates-with-custom-cert <-----using custom defined certification  
  rules:
  - backendRefs:  
    - name: parking
      kind: Service
      port: 8090
    matches:
    - path:
        type: PathPrefix
        value: /parking
  - backendRefs:
    - name: review
      kind: Service
      port: 8090
    matches:
    - path:
        type: PathPrefix
        value: /review
```        