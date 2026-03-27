

# Running Open-Weight LLMs for Student Performance Summaries

## Executive Summary

This document explains how the team can run large language models (LLMs) for student-performance summaries on either:

1. **Cloud-hosted GPUs** using providers such as **Runpod** or **Lambda**, or
2. **On-site purchased hardware** such as **NVIDIA DGX Spark**.

It also explains why the best long-term architecture for our project is:

- **Primary inference on hardware we control on site**, using open-weight models we choose and tune,
- **Cloud GPU as backup and burst capacity**, with **Runpod** and **Lambda** as the primary options to evaluate.

That approach gives us the best balance of:

- predictable cost,
- data control,
- repeatability of outputs,
- owned hardware provides full use 24x7 at no additional cost for maximum application of the resource,
- and resilience when local hardware is unavailable or demand spikes.

---

## Why We Are Evaluating This

We are currently using LLM APIs to generate summaries of student performance in Mission HydroSci. That works, but API cost can rise quickly because summaries are generated repeatedly across students, activities, and reporting cycles.

The question is not just whether an API can do the work. It is whether we can run our own model stack in a way that is:

- affordable,
- privacy-conscious,
- IRB-friendly,
- stable enough to tune once and use repeatedly,
- and operationally realistic for a university research project.

A major lesson from experimentation so far is that **different models can produce different summaries even with the same prompt, context, and data**. That means our real goal should be to **standardize on a primary model environment**, tune that environment carefully, and avoid constant model-switching so the results remain predictable.

This is one of the strongest arguments for owning the primary inference path rather than relying entirely on shifting external APIs.

---

## Recommendation

### Recommended architecture

**Primary:** on-site hardware we control (for example, NVIDIA DGX Spark)

**Backup / burst:** cloud GPUs, with **Runpod** and **Lambda** as the two most relevant providers for evaluation

### Why this is the best fit

1. **Data control is strongest on-site.** Student data stays inside infrastructure we manage directly.
2. **Output consistency is better.** We can pick one model, one quantization strategy, one prompt template, and tune for that environment.
3. **Costs become more predictable.** Hardware is a capital purchase rather than ongoing per-token spend.
4. **Cloud still covers risk.** If local hardware is down, overloaded, or temporarily insufficient, we can fail over to Runpod or Lambda.
5. **The project keeps the asset.** Unlike service spend, purchased hardware remains available after the initial budget period.

In short:

> **Own the primary inference path. Rent backup compute when needed.**

---

## The Main Ways These Systems Can Be Used

There are several distinct operating modes. These are often conflated, but they are materially different.

### 1. API model

This is the current pattern with hosted LLM APIs. We send prompt + context + student data to a vendor-managed model endpoint and get back a summary.

**Pros**
- easiest to start,
- strongest frontier models,
- no infrastructure to manage.

**Cons**
- recurring usage cost,
- changing model behavior over time,
- lower control over the full inference stack,
- more concern from stakeholders about student data leaving project-controlled infrastructure.

### 2. Cloud GPU instance or pod running our own model

Here we rent a GPU machine from a cloud provider, install or launch an inference server such as **vLLM**, and serve an open-weight model ourselves.

**Pros**
- much more control than API use,
- can use open-weight models from Hugging Face and similar sources,
- can expose an OpenAI-compatible API from our own stack,
- often dramatically cheaper per summary than premium APIs.

**Cons**
- we still rely on third-party infrastructure,
- we must manage model serving, storage, access control, and monitoring,
- some providers have capacity variation.

### 3. On-site hardware running our own model

This is the same architecture as the cloud-GPU case, except the hardware is in our own environment.

**Pros**
- strongest privacy/control position,
- stable model environment,
- retained institutional asset,
- no per-request inference bill,
- best fit when we want to optimize one long-lived model stack.

**Cons**
- upfront hardware purchase,
- maintenance responsibility,
- limited local redundancy unless we buy more than one system.

### 4. Hybrid architecture

This is the recommended design.

**Primary path**: on-site inference  
**Secondary path**: cloud failover or cloud burst capacity  
**Optional tertiary path**: premium API only for exceptional cases

This gives us:

- privacy and control by default,
- resilience when hardware is unavailable,
- flexibility if demand grows unexpectedly.

---

## Why Standardizing on One Primary Model Environment Matters

A major operational issue is that:

> the same prompt, context, and data do not produce the same style or quality of summary across different models.

We have already observed this with different hosted models. The same will also be true when switching among:

- API models,
- cloud-hosted open models,
- and local/open models on owned hardware.

This means the real product is not just “the model.” It is a **summary engine**, consisting of:

- the model family,
- quantization level,
- inference runtime,
- prompt template,
- context construction,
- output schema,
- and validation/evaluation rules.

The more we switch model backends, the more tuning and evaluation noise we introduce.

That is one of the strongest reasons to propose:

- **one primary local/on-site environment**, and
- **cloud as backup**, not as a constantly changing primary.

---

## Cloud GPU Provider Landscape

The full GPU-cloud landscape is broad. Major options in 2026 include hyperscalers such as **AWS, Azure, Google Cloud, Oracle Cloud, and IBM Cloud**, plus AI-native providers such as **Runpod, Lambda, CoreWeave, Crusoe, DigitalOcean/Paperspace, Replicate, Vast.ai**, and others. DigitalOcean’s 2026 roundup is a good high-level snapshot of the major players. [^do-providers]

For our purposes, the list of options is useful, but the most relevant providers for serious evaluation are:

- **Runpod**
- **Lambda**

Those are the focus of this document because they map well to our needs:

- open-weight model serving,
- cost-sensitive inference,
- relatively straightforward deployment,
- and realistic use for research workloads.

---

## Runpod Deep Dive

### What Runpod is

Runpod is an AI-focused GPU platform offering both **Pods** and **Serverless** products. Its pricing page distinguishes **Flex** workers, which scale up when needed and return to idle, from **Active** workers, which stay warm to eliminate cold starts. [^runpod-pricing]

### Main operating modes on Runpod

#### A. Pods

A Pod is the closest thing to renting a GPU machine.

You choose a GPU, create a pod, and run your own software stack on it. This is usually the best first step because it is easiest to debug and easiest to treat like a normal Linux machine.

This is ideal when we want:

- a stable inference target,
- SSH access,
- explicit control over the runtime,
- the ability to install or launch **vLLM** ourselves.

#### B. Serverless

Runpod also supports **serverless vLLM** deployments for open-source models. Their documentation explicitly describes deploying vLLM serverlessly, selecting a model, and invoking it through an endpoint. [^runpod-vllm]

This is ideal when we want:

- lower idle cost,
- scale-to-zero behavior,
- an event-driven or batch workflow.

The tradeoff is **cold starts** and somewhat more operational abstraction.

#### C. Flex vs Active workers

Runpod’s pricing distinguishes:

- **Flex**: scale up only when traffic arrives; best for bursty workloads
- **Active**: always-on warm workers; avoids cold starts but costs more [^runpod-pricing]

For our student-summary workflow, this matters because we can choose between:

- **bursty batch processing**, or
- **always-ready serving** during active periods.

### Typical Runpod GPUs relevant to us

Runpod’s official pricing currently lists, among others:

- **A100 80GB**
- **H100 80GB**
- **L40 / L40S / 6000 Ada 48GB**
- **A6000 / A40 48GB**
- **4090 24GB** [^runpod-pricing] [^runpod-gpu-pricing]

For our work, the most relevant classes are:

- **A100 80GB** – safest general choice for LLM inference,
- **L40S 48GB** – potentially attractive lower-cost inference option,
- **H100** – usually more than we need unless throughput becomes very high.

### Runpod pricing we should care about

Runpod publishes both per-second serverless pricing and per-hour GPU pricing.

Examples from official pricing pages:

- Serverless **A100 80GB**: **$0.00076/sec Flex** and **$0.00060/sec Active** [^runpod-serverless]
- Serverless **H100 80GB**: **$0.00116/sec Flex** and **$0.00093/sec Active** [^runpod-serverless]
- On-demand **A100 PCIe 80GB**: **$1.19/hr** [^runpod-gpu-pricing]
- On-demand **H100 PCIe 80GB**: **$1.99/hr** [^runpod-gpu-pricing]

### Runpod storage pricing

Runpod’s official storage pricing is important because model caching matters a lot:

- **Network volume**: **$0.07/GB/month under 1TB**, **$0.05/GB/month over 1TB** [^runpod-storage]
- **Volume disk**: **$0.10/GB/month while running**, **$0.20/GB/month while stopped** [^runpod-storage]
- **Container disk**: temporary, erased when a Pod stops [^runpod-storage]

For our work, the right choice is usually **network volume**, because model weights persist independently of the pod and do not need to be downloaded every time.

### Capacity behavior on Runpod

Runpod is flexible and cost-effective, but it is not the same as guaranteed reserved enterprise capacity.

Their documentation explicitly notes that when a stopped Pod is restarted, it may show **“Zero GPU Pods”** if the original GPU is no longer available. Their Pod migration documentation also explains that a Pod is attached to a specific physical machine while it is running, and that stopping it releases that GPU to the pool. [^runpod-zero] [^runpod-migration]

So the practical takeaway is:

- as long as a Pod is running, the reserved GPU is stable,
- once it is stopped, that exact GPU may no longer be available.

This is manageable for our workload because our workload is **batchable**. We do not need ultra-strict low-latency SLAs. But it is still a reason not to rely on Runpod as the only production path for sensitive project operations.

### Security and privacy posture on Runpod

Runpod’s documentation states:

- Pods and workers run with **containerized isolation** in a **multi-tenant environment**,
- **Secure Cloud** operates in enterprise-grade data centers,
- Runpod’s host policies prohibit hosts from inspecting Pod/worker data,
- Secure Cloud can use vetted infrastructure partners meeting standards including **SOC 2, ISO 27001, and PCI DSS** [^runpod-security]

Runpod also announced **SOC 2 Type II certification** in October 2025. [^runpod-soc2]

### Runpod summary

**Strengths**
- flexible,
- cost-efficient,
- supports pods and serverless,
- straightforward for open-source inference,
- good fit for bursty workloads.

**Weaknesses**
- capacity can vary,
- Secure Cloud and storage choices require deliberate configuration,
- multi-tenant by default unless we choose the right deployment pattern.

---

## Lambda Deep Dive

### What Lambda is

Lambda is an AI-focused cloud company with roots in deep-learning workstations and servers. Today it provides cloud GPUs, on-demand instances, clusters, private cloud, and an enterprise trust/security program. [^lambda-home] [^lambda-pricing]

### Main operating modes on Lambda

#### A. On-Demand Cloud instances

Lambda’s public-cloud documentation describes launching individual Linux-based GPU-backed virtual machines and turning them down on our schedule. [^lambda-public-cloud]

This is the closest counterpart to Runpod Pods.

It is ideal for:

- stable inference servers,
- SSH-based administration,
- running vLLM or Ollama,
- predictable hands-on control.

#### B. 1-Click Clusters

Lambda also offers **1-Click Clusters**, documented as production-ready clusters of **16 to 512 H100 GPUs**. [^lambda-public-cloud] [^lambda-pricing]

This is far more than we need for student-summary generation, but it shows the platform can scale well if our use grows dramatically.

#### C. Private Cloud / enterprise posture

Lambda’s site emphasizes a **single-tenant, shared-nothing architecture**, **SOC 2 Type II certification**, and isolated/caged clusters in enterprise contexts. [^lambda-home]

This is relevant because our team is worried about student data, IRB, and institutional review.

### Lambda pricing we should care about

Lambda’s current pricing page is oriented around instances and clusters. As of the public pricing currently shown:

- **H100 systems** in cluster pricing are shown at **$2.76/hr** [^lambda-pricing]
- Current instance pricing examples shown publicly include:
  - **A100 PCIe 40GB**: **$1.99/hr**
  - **A6000 48GB**: **$1.09/hr**
  - **H100 SXM 80GB**: **$3.99/hr** in the 8-GPU presentation on the pricing page [^lambda-pricing-page-2]

The key point is not any single number; it is that **Lambda is generally positioned as a more stable, more enterprise-oriented AI cloud than Runpod, but usually not the cheapest option**.

### Security and privacy posture on Lambda

Lambda’s official site says:

- **single-tenant, shared-nothing architecture**,
- **SOC 2 Type II certification**,
- and hardware-level isolation options in its enterprise posture. [^lambda-home]

Lambda also provides a **Trust Portal** specifically for security posture and compliance documentation. [^lambda-trust]

### Lambda summary

**Strengths**
- strong research/enterprise positioning,
- stable instance-based model,
- clear security/trust posture,
- good if we want a more conventional GPU cloud.

**Weaknesses**
- usually higher cost than Runpod for comparable flexible usage,
- less oriented toward scale-to-zero burst economics,
- for our use case may be more infrastructure than we need for routine inference.

---

## Runpod vs Lambda

### In one sentence

- **Runpod** is generally better when we want **lower-cost bursty inference and flexibility**.
- **Lambda** is generally better when we want **stable AI-cloud instances and a stronger enterprise/security story**.

### Practical comparison

| Dimension | Runpod | Lambda |
|---|---|---|
| Best mental model | flexible GPU utility | stable AI cloud |
| Primary modes | Pods, Serverless, Flex/Active workers | On-Demand instances, clusters, private cloud |
| Cost profile | often lower, especially for bursty workloads | often higher, but more “traditional cloud” |
| Capacity behavior | can vary; stopped pods can lose GPUs | more instance-oriented and stable |
| Security posture | multi-tenant by default, stronger with Secure Cloud | strong trust/security posture, single-tenant messaging |
| Best fit for us | burst, backup, experiments | backup, stable cloud serving, enterprise discussions |

### Which is better for our project?

If we are choosing a **backup provider** for an on-site primary system:

- **Runpod** is attractive as the **cost-sensitive burst/failover** option.
- **Lambda** is attractive if the team wants a **stronger comfort level around enterprise posture and private-cloud language**.

It is reasonable to test both, but if we standardize on one cloud backup path:

- choose **Runpod** for lower cost and flexibility,
- choose **Lambda** if institutional comfort with security messaging matters more than raw cost.

---

## What Is Involved in Actually Running an LLM on Cloud GPUs

This is the mechanics stakeholders often do not understand.

### The pieces

To run an open-weight LLM on cloud GPUs, we typically need:

1. a GPU machine (Pod, instance, or cluster),
2. a model source,
3. an inference engine,
4. persistent storage for model caching,
5. and an HTTP API surface our application can call.

### Model source

Usually the model weights live in:

- **[Hugging Face Hub](https://huggingface.co/models)**, or
- a local/persistent filesystem once downloaded.

### Inference engine

The most important serving engine for our purposes is **vLLM**, which is designed for high-throughput LLM serving and exposes an **OpenAI-compatible API**. vLLM documents support for a large range of open-source generative and pooling models and explicitly supports deployment on Runpod. [^vllm-supported] [^vllm-runpod]

### Storage

Model weights do **not** remain on the GPU when the system is shut down. Long-term they live in:

- a local disk,
- a persistent cloud volume,
- or a model repository like Hugging Face.

When the instance starts, the model is loaded from storage into memory and then GPU memory.

### Deployment pattern

A typical deployment looks like this:

1. Launch pod/instance.
2. Mount or attach persistent storage.
3. Authenticate to Hugging Face if the model requires a license grant or token.
4. Start `vllm serve` with the chosen model.
5. Expose an internal or protected HTTP endpoint.
6. Send structured student-summary requests to that endpoint.
7. Log outputs and evaluation metadata under our control.

### Why this matters

This architecture is materially different from sending student data directly to an external API vendor’s model endpoint. We are still using third-party infrastructure, but **we control the model, inference server, prompt layer, and retention behavior much more directly**.

---

## Open-Weight Model Sources and What Is Available

### Primary model source: [Hugging Face Hub](https://huggingface.co/models)

The main source for open-weight models today is **[Hugging Face Hub](https://huggingface.co/models)**, which states that it hosts **over 2 million models**. [^hf-hub]

This is the default place to obtain models for self-hosted inference.

### Common sources and registries

#### 1. [Hugging Face Hub](https://huggingface.co/models)

This is the main repository for downloading and versioning model checkpoints. The Hub docs describe model repositories and the Model Hub as the central place to host and discover models. [^hf-hub] [^hf-model-hub]

#### 2. Vendor org pages on Hugging Face

Many model developers publish official releases there, including:

- **Meta Llama** [^meta-llama]
- **Qwen** [^qwen]
- **Mistral AI** [^mistral]
- **Google Gemma** [^gemma]
- **Microsoft Phi** [^phi]

#### 3. Vendor download pages

Some model families also have official vendor download pages or access workflows. For example, Meta provides Llama model access through its official download process. [^llama-download]

#### 4. Ollama library

Ollama is another convenient distribution path for many open models, especially when the goal is easy local experimentation. Ollama’s public library includes many model families and emphasizes local/open use. [^ollama] [^ollama-library]

### Model families likely most relevant to us

For student-performance summaries, we should focus on compact and mid-sized instruct models rather than frontier-scale models.

Strong candidates include:

- **Llama family** (widely supported, common default)
- **Qwen family** (strong open-model performance)
- **Mistral / Ministral family**
- **Gemma family**
- **Phi family**

A vLLM-supported model family is preferable because it keeps deployment simpler. [^vllm-supported]

### Important note on “open source” vs “open weight”

In practice, many teams say “open source models” when they really mean **open-weight models**. The most important practical question for us is:

- can we download the weights,
- run them ourselves,
- and keep inference under our own control?

For this project, that matters more than the semantic distinction.

---


## Can Purchased On-Site Hardware Actually Do the Job?

---

## Model Size, GPU Requirements, and Quantization

This section explains how model size, GPU memory, and quantization affect what we can realistically run on:

- on-site hardware such as DGX Spark, and
- cloud GPUs from providers like Runpod and Lambda.

Understanding this is critical because it directly answers:

- what models we can run,
- how much hardware we need,
- and whether on-site inference is viable for our use case.

### Model Size vs GPU Memory (High-Level Guide)

Approximate memory requirements for common model sizes:

| Model Size | VRAM (FP16) | VRAM (4-bit quantized) | Typical Deployment |
|-----------|-------------|------------------------|--------------------|
| 7B | ~14 GB | ~4–6 GB | Runs anywhere (local, DGX, cloud) |
| 13B | ~26 GB | ~8–10 GB | Ideal for DGX and most GPUs |
| 30B | ~60 GB | ~15–20 GB | DGX (quantized) or A100-class GPUs |
| 70B | ~140 GB | ~35–45 GB | Multi-GPU or high-end cloud only |

These are approximate ranges, but they are sufficient for planning and decision-making.

### What This Means for DGX Spark

For a system like **DGX Spark (single GPU class system)**:

- **Comfortable range:**
  - 7B–13B models (full precision or quantized)
- **Practical upper range:**
  - ~30B models (when quantized, e.g., 4-bit)
- **Not ideal for:**
  - 70B models unless heavily optimized or distributed

For our use case (structured student-performance summaries), the 7B–30B range is typically sufficient.

### What This Means for Cloud GPUs (Runpod / Lambda)

Typical cloud GPU options include:

- A100 (80GB)
- L40S (48GB)
- H100 (80GB)

These allow:

- **7B–30B models easily**, often without aggressive quantization
- **70B models with quantization or multi-GPU setups**
- scaling across multiple GPUs if needed

Cloud GPUs therefore provide a path to larger models, but that does not necessarily translate into better outcomes for our specific task.

### What Quantization Is

Quantization is the process of reducing the numerical precision of model weights in order to:

- reduce memory usage,
- increase speed,
- and allow larger models to run on smaller hardware.

### Common Quantization Levels

| Quantization | Memory Usage | Quality Impact |
|-------------|-------------|----------------|
| FP16 (full precision) | 100% | highest fidelity |
| 8-bit | ~50% | nearly identical in most cases |
| 4-bit | ~25% | small quality reduction |
| 2-bit | ~12% | noticeable degradation |

### Recommended Approach for This Project

For student-performance summaries:

- **4-bit quantization is typically the best balance** of:
  - memory efficiency,
  - speed,
  - and output quality

It allows:

- larger models to fit on DGX-class hardware,
- lower cost operation in cloud environments,
- and sufficient quality for structured summarization tasks.

### Key Insight for the Team

The most important takeaway is:

> For structured summarization tasks, smaller quantized models (7B–30B) often perform sufficiently well, making them practical for on-site deployment without requiring large-scale GPU infrastructure.

This is why our proposed architecture does **not** depend on frontier-scale models.

Instead, it depends on:

- consistent model selection,
- careful prompt design,
- structured input data,
- and a stable inference environment.

### Why This Supports the Proposed Architecture

Because:

- DGX Spark can run the model sizes we need,
- quantization makes those models efficient and practical,
- cloud GPUs remain available for larger or overflow workloads,

we can confidently adopt:

- **on-site inference as the primary path**, and
- **cloud GPU as backup and scaling support**.

---

### The key question

Can a system like **DGX Spark** run quantized open-weight models well enough to produce good student performance summaries?

### Practical answer

Yes—very likely.

The summarization task we care about is not the same as frontier general reasoning. We are not asking the model to solve open-ended research problems. We are asking it to:

- digest structured student-performance data,
- identify strengths and struggles,
- and produce clear teacher-facing language.

That is a good fit for smaller and mid-sized instruct models.

### Why hardware can work well here

Student summaries are especially compatible with:

- smaller instruct models,
- 4-bit or similar quantization,
- careful prompt design,
- and structured input rather than raw logs.

The biggest quality lever is often not raw model size; it is:

- input organization,
- prompt design,
- and consistent deployment.

That reinforces the case for **one primary on-site model stack**.

---

## Approximate Cost Thinking

These are not procurement quotes; they are planning-level approximations based on the classes of GPU and workflows we discussed.

### Cloud GPU cost characteristics

For our kind of workload, per-summary cost on cloud GPU can be very low when we:

- batch requests,
- keep output lengths controlled,
- use an appropriate open model,
- and avoid premium API pricing.

The main variables are:

- model size,
- actual input length,
- output token count,
- and whether the endpoint is warm or cold.

### Why cloud is still useful even if we buy hardware

Even if the long-term plan is on-site inference, cloud remains useful for:

- failover,
- overflow workloads,
- trying models before standardizing,
- and benchmarking.

### Why on-site becomes attractive over time

For recurring workloads, owned hardware changes the economics from:

- “pay every time the model runs”

into:

- “pay once for the machine, then mostly pay power/ops.”

This is especially attractive when the institution can keep using the hardware after the initial project period.

---

## Data Privacy, IRB, and Student Data

This is one of the most important sections for internal discussion.

### The strongest privacy position

The strongest privacy position is:

- **run the model on-site**,
- on hardware we control,
- inside network and access controls we already manage.

That gives the clearest answer to concerns like:

- Where did the student data go?
- Who had access to it?
- Was it sent to a public API?
- Was it retained in someone else’s system?

### Why cloud-hosted open models are different from API use

Using a cloud GPU to run our own open-weight model is **not the same** as using a third-party hosted LLM API.

With cloud-hosted open models:

- we choose the model,
- we run the inference server,
- we can minimize or disable application-side logging,
- we can control what is persisted,
- and there is no automatic model training on our prompts unless we explicitly build a fine-tuning pipeline.

However, it is still true that:

- the infrastructure belongs to a third party,
- the deployment must be configured correctly,
- and institutional review may still require a formal privacy/security review.

### Runpod privacy/security notes

Runpod states that:

- Pods/workers are isolated with containers,
- Secure Cloud uses enterprise-grade data centers,
- host access to customer Pod/worker data is prohibited by policy,
- and Secure Cloud can be constrained to vetted infrastructure partners meeting compliance expectations. [^runpod-security]

That is helpful, but because Runpod is multi-tenant by default, **we should treat Secure Cloud as the appropriate Runpod mode for student data**.


### Lambda privacy/security notes

Lambda states that it offers:

- **single-tenant, shared-nothing architecture**,
- **SOC 2 Type II certification**,
- and a trust portal for security documentation. [^lambda-home] [^lambda-trust]

This gives Lambda a particularly strong story for institutional discussions about controlled deployment.

### What SOC 2 Type II Certification Means — and What It Does Not Mean

Because both Runpod and Lambda reference **SOC 2 Type II certification**, it is important to explain what that actually means in practical terms for our team.

#### What SOC 2 Type II is

SOC 2 is an audit framework developed by the **American Institute of Certified Public Accountants (AICPA)** for evaluating controls related to the **security, availability, processing integrity, confidentiality, and privacy** of a service organization. A **Type II** report does not just check whether controls are described on paper; it evaluates whether those controls were **operating effectively over a period of time**. [^aicpa-soc2] [^aws-soc2]

In plain language, a SOC 2 Type II certification or report generally means that an independent auditor has reviewed the provider’s operational controls and concluded that the controls were functioning as designed during the audit period.

#### What SOC 2 Type II does help with

For our purposes, a SOC 2 Type II posture is useful because it indicates that the provider has undergone outside review of its controls around areas such as:

- access control,
- change management,
- incident response,
- system monitoring,
- and data handling procedures.

That can be helpful in conversations with:

- university IT,
- research administration,
- compliance reviewers,
- and project stakeholders who want to know whether a provider is operating with recognized security controls.

In other words, SOC 2 Type II is a **positive trust signal**. It is evidence that the provider is taking operational security seriously and has had that posture reviewed by an independent auditor.

#### What SOC 2 Type II does **not** mean

SOC 2 Type II does **not** mean any of the following by itself:

- that the service is automatically approved for our student data,
- that it is automatically IRB-approved,
- that it is automatically FERPA-compliant for our exact use case,
- that data cannot be exposed if we misconfigure the service,
- or that institutional review is unnecessary.

This is extremely important.

A provider can have strong audited controls and still not be the right place to put identifiable student data unless:

- we configure the service correctly,
- we minimize the data we send,
- our institution approves the use,
- and the research/data-governance requirements are satisfied.

#### How SOC 2 Type II relates to student data and IRB concerns

For our student-data use case, SOC 2 Type II should be viewed as:

- **helpful**,
- **reassuring**,
- and **relevant**,

but **not sufficient on its own**.

It strengthens the case that a provider like Lambda or Runpod Secure Cloud may be suitable as a controlled backup environment, but it does not replace project-level review of:

- whether identifiable student data is being sent,
- what fields are included,
- whether the data can be de-identified or minimized,
- whether logs are retained,
- where the data is processed and stored,
- and whether the institutional or IRB approval path allows that deployment.

#### Practical interpretation for this project

The safest interpretation for our project is:

> SOC 2 Type II helps support the argument that a provider has mature operational controls, but it does **not** by itself answer the question of whether our specific student-data workflow is approved.

That means the right operational stance is still:

1. keep the **primary** summarization path on-site when possible,  
2. use cloud as **backup or overflow**,  
3. send the **minimum necessary data**,  
4. prefer structured derived signals over raw student records,  
5. and confirm that the proposed workflow is acceptable under institutional privacy and IRB expectations.

#### Bottom line

SOC 2 Type II certification is best understood as a **security/compliance maturity signal**, not as an automatic green light for sensitive student-data processing.

It makes providers like Lambda and Runpod more credible options for controlled backup infrastructure, but it does not weaken the case for our preferred architecture:

- **on-site primary inference for maximum data control**, and
- **cloud backup only under carefully chosen and reviewed conditions**.

### Practical privacy recommendations

If student data is involved, our operating assumptions should be:

1. **Primary summaries run on-site whenever possible.**
2. **Cloud is backup, not default.**
3. **If cloud is used, use the provider’s more secure configuration** (Runpod Secure Cloud, Lambda single-tenant/private options where appropriate).  
4. **Send only the minimum required data** to the summarization service.  
5. **Prefer structured performance signals over raw logs** when possible.  
6. **Log and retain outputs deliberately**, not by default.  
7. **Review IRB / institutional requirements before production use of student records in cloud environments.**

### Bottom line on privacy

If stakeholders are worried, the most responsible message is:

> We can get the benefits of LLM summarization **without making cloud or public APIs the default home of student data**.

That is the strongest reason to prefer **on-site first, cloud second**.

---

## Why the Hybrid On-Site + Cloud-Backup Architecture Is Best

### It solves the consistency problem

We can tune prompts and context for one primary model environment and keep that stable.

### It solves the privacy problem

Sensitive student data stays on-site by default.

### It solves the reliability problem

If on-site hardware is unavailable, we can fail over to Runpod or Lambda.

### It solves the procurement/value problem

The institution retains the hardware asset while still preserving operational flexibility.

### It solves the cost problem

We avoid making every summary depend on a paid external API call.

### It solves the growth problem

If demand spikes, cloud covers overflow.

---

## Proposed Architecture for the Team

### Primary environment

- On-site DGX Spark or similar purchased hardware
- One standardized open-weight instruct model
- One inference engine (preferably **vLLM** for the production-style stack)
- One carefully tuned prompt/context pipeline
- Internal protected API endpoint for the summarization service

### Secondary / failover environment

- Runpod or Lambda
- same model family if possible,
- same prompt contract,
- same output schema,
- same evaluation checks.

### Operational pattern

1. Student performance data is preprocessed into structured features/signals.
2. Summarization requests go to the on-site inference service first.
3. If on-site inference is unavailable or backlogged, the request is routed to the backup cloud environment.
4. Summaries are stored under our normal project controls.

### Why this is the right proposal

This architecture is the best balance of:

- privacy,
- technical realism,
- cost control,
- and continuity of service.

---

## Concrete Recommendation to Present to the Team

We should propose the following:

> **Run open-weight LLMs primarily on-site on purchased hardware, and maintain a cloud-based backup/failover path using Runpod or Lambda.**

### Recommendation details

- Use **on-site hardware as the default inference environment**.
- Standardize on **one primary open-weight model family** and optimize prompts for that environment.
- Use **Runpod** as the most cost-flexible burst/backup option.
- Evaluate **Lambda** as the more enterprise/security-oriented backup option.
- Avoid making premium hosted APIs the primary system of record for student summaries.
- Treat cloud as **backup and overflow**, not default.

### Why this is the best approach

Because it gives us:

- a repeatable summary engine,
- stronger privacy positioning,
- long-term cost control,
- retained institutional value,
- and resilience when local hardware is insufficient or down.

This is the most defensible architecture technically, operationally, and institutionally.

---

## Sources

[^do-providers]: DigitalOcean, “10 Leading AI Cloud Providers for Developers in 2026.” https://www.digitalocean.com/resources/articles/leading-ai-cloud-providers
[^runpod-pricing]: Runpod pricing page. https://www.runpod.io/pricing
[^runpod-gpu-pricing]: Runpod GPU pricing page. https://www.runpod.io/gpu-pricing
[^runpod-serverless]: Runpod serverless pricing docs. https://docs.runpod.io/serverless/pricing
[^runpod-storage]: Runpod storage/network volume pricing docs. https://docs.runpod.io/pods/pricing and https://docs.runpod.io/storage/network-volumes
[^runpod-vllm]: Runpod serverless vLLM docs. https://docs.runpod.io/serverless/vllm/get-started
[^runpod-security]: Runpod security/compliance docs. https://docs.runpod.io/references/security-and-compliance
[^runpod-soc2]: Runpod SOC 2 Type II announcement. https://www.runpod.io/blog/runpod-achieves-soc-2-type-ii-certification
[^runpod-zero]: Runpod “Zero GPU Pods on restart.” https://docs.runpod.io/pods/troubleshooting/zero-gpus
[^runpod-migration]: Runpod Pod migration docs. https://docs.runpod.io/pods/troubleshooting/pod-migration
[^lambda-home]: Lambda home page / trust & security positioning. https://lambda.ai/
[^lambda-pricing]: Lambda pricing page. https://lambda.ai/pricing
[^lambda-pricing-page-2]: Lambda pricing page, public instance examples. https://lambda.ai/pricing
[^lambda-public-cloud]: Lambda public cloud docs. https://docs.lambda.ai/public-cloud/
[^lambda-trust]: Lambda Trust Portal. https://trust.lambda.ai/
[^vllm-supported]: vLLM supported models docs. https://docs.vllm.ai/en/stable/models/supported_models/
[^vllm-runpod]: vLLM Runpod deployment docs. https://docs.vllm.ai/en/latest/deployment/frameworks/runpod/
[^hf-hub]: Hugging Face Hub documentation. https://huggingface.co/docs/hub/index
[^hf-model-hub]: Hugging Face Model Hub docs. https://huggingface.co/docs/hub/en/models-the-hub
[^meta-llama]: Meta Llama on Hugging Face. https://huggingface.co/meta-llama
[^qwen]: Qwen official Hugging Face org. https://huggingface.co/Qwen
[^mistral]: Mistral AI official Hugging Face org. https://huggingface.co/mistralai
[^gemma]: Google Gemma examples on Hugging Face / AI Google docs. https://huggingface.co/google/gemma-3-4b-it and https://ai.google.dev/gemma/docs/core/huggingface_inference
[^phi]: Microsoft Phi examples on Hugging Face. https://huggingface.co/microsoft/phi-4 and https://huggingface.co/microsoft/Phi-3-mini-4k-instruct
[^ollama]: Ollama home page. https://ollama.com/
[^ollama-library]: Ollama model library. https://ollama.com/library
[^llama-download]: Meta official Llama downloads. https://www.llama.com/llama-downloads/