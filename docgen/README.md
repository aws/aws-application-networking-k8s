# Generate API Docs

Install doc generator

```sh
go install github.com/ahmetb/gen-crd-api-reference-docs
```

Generate html docs

``` sh
cd docgen

gen-crd-api-reference-docs -config config.json -api-dir "../pkg/apis/applicationnetworking/v1alpha1/" -out-file docs.html
```

Add generated content to template

``` sh
cat api-reference-base.md docs.html > ../docs/api-reference.md
```
