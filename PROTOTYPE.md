> **How can we create a shared language with the provider to express the necessary migrations?**

As a starting point, we should explore the current way that providers express anything to Terraform core, which is via the GRPC provider protocol.

Once that has been explored, we can break outside the protocol and ask what other wayys provider developers (or others!) might want to express
migrations that relate to providers.
    - For example, could HashiCorp define code migrations for the GitHub provider? Which we do not own the source code for? :thinking: