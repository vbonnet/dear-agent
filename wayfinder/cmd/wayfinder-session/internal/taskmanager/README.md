# Wayfinder roadmap tasks

Tasks are stored in the `roadmap` section of `WAYFINDER-STATUS.md` and use a
named phase prefix such as `BUILD-1`.

```sh
wayfinder session task add BUILD "Implement endpoint" --effort 4 --priority P0
wayfinder session task update BUILD-1 --status in-progress
wayfinder session task list --phase BUILD
wayfinder session task show BUILD-1
wayfinder session task delete BUILD-1
```

Run `wayfinder session task <command> --help` for current flags. The manager
validates phase names, task states, dependencies, duplicate IDs, cycles, and
deletion of referenced tasks. It writes through the same canonical status
serializer as lifecycle commands.

Normative behavior is in [SPEC.md](SPEC.md).
