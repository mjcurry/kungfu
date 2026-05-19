<p align="center">
  <img src="kungfu-logo.png" alt="kungfu" width="220">
</p>

# kungfu

> "I know kung fu." — manage AI agent skills from the command line.

`kungfu` is a CLI for managing **skills**: directories containing a `SKILL.md`
file that teach an AI agent a new capability via the progressive-disclosure
pattern used by Claude and similar agents.

<p align="center">
  <img src="loading-skills.gif" alt="Loading skills" width="480">
</p>

> 🚧 **Under construction.** This is the first of a planned series of PRs.
> Today it ships the project scaffold, configuration, and the core skill
> domain model — only the `version` command exists so far. More commands are
> on the way.

<p align="center">
  <img src="i-know-kungfu.gif" alt="I know kung fu" width="320">
</p>

## Documentation

- [The SKILL.md format](docs/skill-format.md) — required and optional fields,
  directory layout, and a complete example.

## Building

```sh
make build      # -> ./bin/kungfu
./bin/kungfu version
```

## Author

[Mike Curry](https://github.com/mjcurry)

## License

[MIT](LICENSE)
