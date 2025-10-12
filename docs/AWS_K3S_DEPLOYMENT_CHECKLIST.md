# AWS k3s Deployment Checklist — NRG‑CHAMP
*(Copy this file into your repo as `AWS_K3S_DEPLOYMENT_CHECKLIST.md` and check items as you go.)*

> Target: Deploy the NRG‑CHAMP microservices to a single‑node **k3s** cluster running on **AWS Learner Lab**.  
> Scope: No code changes—focus on cluster setup, container registry, manifests, secrets, storage, and smoke tests.

---

## 0) Quick Summary (TL;DR)
- Spin up 1 EC2 (Ubuntu 22.04, t3.medium), open ports 22/80/443.
- Install **k3s** (single node), export kubeconfig.
- Push all service images to **ECR** (or Docker Hub).
- `helm install` **Kafka** (Bitnami, single replica).
- Apply `Namespace`, `ConfigMaps`, `Secrets`, `Deployments`, `Services`.
- Expose HTTP APIs via **Traefik Ingress**.
- Run smoke tests (pods healthy, endpoints live, Kafka topics flowing).

---

## 1) Prerequisites
- [ ] AWS Learner Lab account activated and a region selected (e.g., `eu-west-1`).
- [ ] Local CLI tools installed: `aws`, `kubectl`, `helm`, `docker`, `git`.
- [ ] A container registry chosen:
  - [ ] **ECR** (preferred on AWS) **or**
  - [ ] **Docker Hub** (simple).

> Tip: authenticate once and script the build/push loop across services.

---

## 2) Repository Readiness (Local)
- [ ] Dockerfiles build on both macOS and Linux (no absolute local paths).
- [ ] For each service (Aggregator, MAPE, Ledger, Gamification, Topic‑Init, Zone‑Simulator, Assessment, etc.):
  - [ ] Has a `k8s/<service>/deployment.yaml` and `k8s/<service>/service.yaml` (ClusterIP).
  - [ ] Reads configuration from env/flags; **no baked secrets**.
- [ ] Global k8s resources exist under `k8s/base/`:
  - [ ] `namespace.yaml` (e.g., `nrg`).
  - [ ] `configmap.yaml` (non‑secret configs).
  - [ ] `secrets.yaml` (optional; or create via CLI).
  - [ ] `ingress.yaml` (Traefik, host/path rules).
- [ ] Persistent needs identified:
  - [ ] Ledger / other data writers use PVCs (if persistence required).
  - [ ] Logs go to stdout (Kubernetes logs) or ephemeral `emptyDir`.

---

## 3) Build & Push Images
### Option A — Amazon ECR
1. [ ] Create ECR repos (one per service) in your region.
2. [ ] Login:
   ```bash
   aws ecr get-login-password --region <REGION> |    docker login --username AWS --password-stdin <ACCOUNT_ID>.dkr.ecr.<REGION>.amazonaws.com
   ```
3. [ ] Build & push (repeat per service):
   ```bash
   SVC=aggregator
   docker build -t <ACCOUNT_ID>.dkr.ecr.<REGION>.amazonaws.com/nrg-$SVC:$(git rev-parse --short HEAD) services/$SVC
   docker push  <ACCOUNT_ID>.dkr.ecr.<REGION>.amazonaws.com/nrg-$SVC:$(git rev-parse --short HEAD)
   ```

### Option B — Docker Hub
- [ ] `docker login`
- [ ] Build & push:
  ```bash
  USER=<dockerhub_user>; SVC=aggregator
  docker build -t $USER/nrg-$SVC:$(git rev-parse --short HEAD) services/$SVC
  docker push  $USER/nrg-$SVC:$(git rev-parse --short HEAD)
  ```

> Pin the tag in your k8s manifests to the pushed image digest or Git SHA for traceability.

---

## 4) Provision AWS VM
- [ ] Launch EC2 **Ubuntu 22.04 LTS** **t3.medium** (≥30 GB gp2 root).
- [ ] **Security Group** inbound rules:
  - [ ] `22/tcp` (SSH)
  - [ ] `80/tcp` (HTTP)
  - [ ] `443/tcp` (HTTPS)
- [ ] Associate a public IP (default in public subnet).

---

## 5) Install k3s (on the EC2)
SSH to the instance and run:
```bash
curl -sfL https://get.k3s.io | sh -
sudo kubectl get nodes -o wide
# k3s kubeconfig lives at:
sudo cat /etc/rancher/k3s/k3s.yaml
```

- [ ] Copy `/etc/rancher/k3s/k3s.yaml` locally as `~/.kube/config-k3s-aws` and set:
  ```bash
  export KUBECONFIG=~/.kube/config-k3s-aws
  kubectl get ns
  ```

> Tip: Change the `server:` address in the kubeconfig to the EC2 **public IP** or DNS name.

---

## 6) Helm Setup & Kafka
- [ ] Install Helm (locally) and add Bitnami repo:
  ```bash
  helm repo add bitnami https://charts.bitnami.com/bitnami
  helm repo update
  ```
- [ ] Create working namespace:
  ```bash
  kubectl create ns nrg
  ```
- [ ] Install single‑replica Kafka (dev profile):
  ```bash
  helm install kafka bitnami/kafka     --namespace nrg     --set replicaCount=1     --set zookeeper.replicaCount=1     --set kraft.enabled=false     --set listeners.client.protocol=PLAINTEXT     --set advertisedListeners.client.protocol=PLAINTEXT
  ```
- [ ] Confirm readiness:
  ```bash
  kubectl -n nrg get pods
  ```

> If memory is tight, add `--set resources.requests.memory=512Mi` etc., or consider Kraft‑only mode in newer charts.

---

## 7) Cluster Configuration (Namespace, Config, Secrets)
- [ ] Apply namespace (if not already):
  ```bash
  kubectl apply -f k8s/base/namespace.yaml
  ```
- [ ] Apply **ConfigMap** (non‑secret env like Kafka bootstrap `kafka:9092`, topic names, service ports):
  ```bash
  kubectl -n nrg apply -f k8s/base/configmap.yaml
  ```
- [ ] Create **Secrets** (recommended via CLI so values don’t live in Git):
  ```bash
  kubectl -n nrg create secret generic ledger-creds     --from-literal=LEDGER_USER=<user>     --from-literal=LEDGER_PASSWORD=<pass>
  ```
  or apply `k8s/base/secrets.yaml` if you manage via sealed‑secrets or SOPS.
- [ ] (Optional) **Storage**: If any service needs persistence, define a StorageClass + PVC backed by EBS.
  - Minimal dev option: use `hostPath` or `emptyDir` (non‑persistent).

---

## 8) Deploy Platform Services
For each service (repeat):
- [ ] Review image reference in `k8s/<service>/deployment.yaml`.
- [ ] Ensure env vars reference ConfigMap/Secret keys.
- [ ] Apply manifests:
  ```bash
  kubectl -n nrg apply -f k8s/<service>/
  ```
- [ ] Verify pod readiness:
  ```bash
  kubectl -n nrg get deploy,pods,svc
  ```

**Typical service ordering (suggested):**
1. Topic‑Init (if it seeds Kafka topics)
2. Aggregator (producer)
3. MAPE (consumer+producer)
4. Ledger (consumer, persistence)
5. Gamification (consumer/UI or API)
6. Zone‑Simulator / Assessment (as needed)

---

## 9) Ingress (HTTP entry via Traefik)
k3s ships with **Traefik** by default.
- [ ] Create/adjust `k8s/base/ingress.yaml` with host/path routing to services:
  ```yaml
  apiVersion: networking.k8s.io/v1
  kind: Ingress
  metadata:
    name: nrg-ingress
    namespace: nrg
    annotations:
      kubernetes.io/ingress.class: traefik
  spec:
    rules:
    - host: <your-public-dns-or-ip>.nip.io
      http:
        paths:
        - path: /gamification
          pathType: Prefix
          backend:
            service:
              name: gamification
              port:
                number: 80
  ```
- [ ] Apply and test:
  ```bash
  kubectl -n nrg apply -f k8s/base/ingress.yaml
  kubectl -n nrg get ingress
  curl -i http://<your-public-dns-or-ip>.nip.io/gamification/health
  ```

> Tip: `nip.io`/`sslip.io` give DNS on the fly from an IP (handy for Learner Lab).

---

## 10) Smoke Tests
- [ ] All pods are `Running` and deployments have desired replicas:
  ```bash
  kubectl -n nrg get deploy,pods
  ```
- [ ] Service endpoints respond:
  ```bash
  kubectl -n nrg port-forward svc/gamification 8080:80
  curl -i http://localhost:8080/health
  ```
- [ ] Kafka end‑to‑end:
  - [ ] Topics exist (from Topic‑Init).
  - [ ] Aggregator produces messages (logs show sends).
  - [ ] MAPE consumes/produces to downstream.
  - [ ] Ledger consumes and persists (or logs).
  ```bash
  # Example: list topics inside the Kafka pod
  kubectl -n nrg exec -it deploy/kafka -- bash -lc "kafka-topics.sh --list --bootstrap-server kafka:9092"
  ```

---

## 11) Observability (Optional but Recommended)
- [ ] Basic: use `kubectl logs -f` and readiness/liveness probes.
- [ ] Stack: deploy Prometheus + Grafana via Helm (light profiles).
- [ ] Central logs: Loki + Promtail (or Fluent Bit) for multi‑service log search.

---

## 12) Security & Cost Controls
- [ ] Do **not** hardcode secrets in Git; use k8s Secrets (or Sealed Secrets/SOPS).
- [ ] Restrict SG to only required inbound ports (22/80/443).
- [ ] Use IAM least privilege for ECR pulls if using IRSA (optional).
- [ ] **Learner Lab expiry**: snapshot notes, export manifests, stop/terminate EC2 to avoid costs when done.

---

## 13) Rollback & Recovery
- [ ] Keep a manifest bundle per release (Git tag).  
- [ ] Rollback by re‑applying previous image tags in deployments:
  ```bash
  kubectl -n nrg rollout history deploy/<service>
  kubectl -n nrg rollout undo deploy/<service> --to-revision=<n>
  ```
- [ ] Backups: if using persistence, snapshot EBS or back up PVC data.

---

## 14) Troubleshooting Quick Reference
- **Image pull errors** → Check registry auth/URL/tag; try `kubectl describe pod` for events.
- **CrashLoopBackOff** → `kubectl logs <pod>`; verify env vars; check Kafka bootstrap & topics.
- **Kafka not reachable** → use in‑cluster DNS (`kafka:9092`), confirm service name/port; check pod resources.
- **Ingress 404** → correct `kubernetes.io/ingress.class: traefik`; verify host/path; `kubectl get ingress` status.
- **Out of resources** → bump EC2 size or reduce replicas/resource requests.

---

## 15) Final Go‑Live Checklist
- [ ] ✅ k3s node healthy and reachable.
- [ ] ✅ Images pushed and referenced by immutable tags.
- [ ] ✅ Namespace/ConfigMap/Secrets applied.
- [ ] ✅ Kafka (and ZK or Kraft) up, topics seeded.
- [ ] ✅ All services deployed and healthy.
- [ ] ✅ Ingress routes live over HTTP(S).
- [ ] ✅ Smoke tests pass (HTTP + Kafka flow).
- [ ] ✅ Credentials stored as Secrets, not in Git.
- [ ] ✅ Notes on how to recreate (commands/scripts) committed to docs.

---

### Appendix: Useful One‑Liners
- Delete and re‑deploy a single service quickly:
  ```bash
  SVC=aggregator
  kubectl -n nrg delete -f k8s/$SVC/ --ignore-not-found
  kubectl -n nrg apply  -f k8s/$SVC/
  kubectl -n nrg rollout status deploy/$SVC
  ```
- Tail logs for many pods:
  ```bash
  kubectl -n nrg get pods -o name | xargs -I{} kubectl -n nrg logs -f {} --max-log-requests=10
  ```
