## Helm install for randomfail chart
This helm script installs the randmon fail deployment within a kubernetes cluster.

```bash
helm install randomfail . -n randomfail --create-namespace
```

Ohne cert-manager im Cluster (z. B. lokales kind) das Certificate abschalten:

```bash
helm install randomfail . -n randomfail --create-namespace --set certManager.enabled=false
```

```bash
kubectl get gateway,virtualservice -n randomfail
```

```bash
helm upgrade randomfail . -n randomfail 
```

```bash
helm uninstall randomfail -n randomfail
```
