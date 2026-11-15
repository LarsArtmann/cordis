// Example smoke app for the TypeScript original: a Service-provided
// greeter resolved off the context by its registered name. Run from the
// repo root after `yarn install && yarn build core`:
//
//   node examples/typescript/greeter.mjs
//
import { Context, Service } from "cordis";

class Greeter extends Service {
  constructor(ctx) {
    super(ctx, "greeter");
  }

  greet(who) {
    return `Hello, ${who}, from cordis!`;
  }
}

const root = new Context();
await root.plugin(Greeter);
console.log(root.greeter.greet("world"));
