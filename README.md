# Which kernel metrics should be used for monitoring memory usage?

Memory management is one of the most intricate and fascinating subsystems in the Linux kernel. With its fine-grained states and various accounting methods, it offers many ways to measure and interpret memory usage—often leading to confusion.

Most of Linux kernel's memory states can be tracked via the `/proc/meminfo` file:

```
$ cat /proc/meminfo
MemTotal:       32512140 kB
MemFree:        16227240 kB
MemAvailable:   23151880 kB
Buffers:         2107068 kB
Cached:          5616492 kB
...
```


The /proc/meminfo file doesn’t provide a single, clear memory utilization value. Instead, it lists multiple fields that appear to represent similar concepts—so which one should you trust? Which ones should you use to calculate the actual free memory?

**Should you rely on MemFree? Or maybe MemAvailable? What about summing MemFree + Cached + Buffers, assuming all of them are "freeable" states?**

Before Linux kernel 3.14, this was a common source of confusion, often leading to misleading or inconsistent memory usage calculations. To address this, the kernel introduced a new metric: `MemAvailable`. MemAvailable was specifically added to provide a more accurate estimate of how much memory is actually available for applications, without requiring users to understand the internals of the kernel's memory management. Introduction to the kernel: https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/commit/?id=34e431b0ae398fc54ea69ff85ec700722c9da773

Monitoring tools like `free`, `top` or `ps` switched to rely on `MemAvailable` since version `v4.0.1`, [reference commit](https://gitlab.com/procps-ng/procps/-/commit/2184e90d2). On the other hand, many monitoring agents still rely on old usage formula: `Used = Total - Free - Cached - Buffers - Sreclaimable`.

## Why is important using the right metrics?

**There is an average difference of 6% between the two formulas, which is not negligible.** This means that depending on the formula used, one might mistakenly believe there is free memory available, when in fact it could trigger the OOM killer.

From a sysadmin perspective, relying on inaccurate metrics can lead to improper memory provisioning—either over-allocating resources and wasting costly hardware, or under-allocating and risking unexpected crashes and downtime.

## Try it out!

Two main containers are provided to verify the actual free memory:

- [memgenerator](./memgenerator/): Allocates `Total - Free - Cached - Buffers - Sreclaimable` bytes of memory, or `MemAvailable` if the `useAvailable` flag is set. It reports the system's memory stats every 20 seconds.
- [oomwatcher](./oomwatcher/): Hooks an eBPF trace to the kernel's oom_kill_process function in order to observe which processes are being OOM killed and print context information.

The following commands will disable the system's swap* and launch the
corresponding containers using Docker compose:

```bash
sudo swapoff -a && { sudo docker compose up; sudo swapon -a; }
```

* Disabling swap ensures that all memory usage is strictly from physical RAM, preventing the system from offloading inactive pages to disk.
