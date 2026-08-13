"use strict";

const cause = Object.assign(
  new Error("listen EADDRINUSE: address already in use 127.0.0.1:8080"),
  { code: "EADDRINUSE", address: "127.0.0.1", port: 8080 },
);

process.stderr.write("starting checkout listener on 127.0.0.1:8080\n");
throw new Error("checkout listener cannot bind its configured endpoint", {
  cause,
});
