# Installed interface descriptions

reference.mjs only reads package-local contracts/interfaces.json and recognizes method-help requests. It has no Task state, workspace behavior, authorization decisions or process execution.

Edit the owning Core/Host contract and example producer, then run scripts/package-interface-contracts.mjs to copy the current reference reader and descriptions into the four Host packages and unified launcher. Generated lib/interface-reference.mjs files are not edited directly. The package manifest includes both files; installed-only tests run without Core, Git or a data directory.

Core method help describes the contract shipped with the package. Actual invocation must use the matching Core; server_info reports the connected runtime's version and current tools. Publication and installation support remain governed by the existing release contracts.
