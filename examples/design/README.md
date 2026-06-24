# Design Workflow Examples

These requests are first-milestone `kicadai design create` inputs. They use
explicit circuit blocks rather than natural-language planning.

Run:

```sh
kicadai --json --request examples/design/led_indicator.json --output /tmp/kicadai-led --overwrite design create
```

The LED example targets structural acceptance and skips board routing. The
sensor breakout example asks for connectivity and is expected to produce useful
placement/routing/validation feedback as the workflow matures.
