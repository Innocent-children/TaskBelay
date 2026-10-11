#!/usr/bin/env node

import { realpathSync } from "node:fs";
import { pathToFileURL } from "node:url";

import { runMain } from "../lib/lifecycle.mjs";
import { runPet } from "../lib/pet.mjs";
import { runTaskBelay } from "../lib/runtime.mjs";

function isMainModule() {
  if (!process.argv[1]) return false;
  try {
    return pathToFileURL(realpathSync(process.argv[1])).href === import.meta.url;
  } catch {
    return false;
  }
}

if (isMainModule()) {
  const arguments_ = process.argv.slice(2);
  const result = arguments_[0] === "pet"
    ? await runPet(arguments_)
    : ["webui","call"].includes(arguments_[0]) || ["help", "--help", "-h", "version", "--version"].includes(arguments_[0])
      ? await runTaskBelay(arguments_)
      : await runMain(arguments_);
  process.exitCode = result.code;
}
