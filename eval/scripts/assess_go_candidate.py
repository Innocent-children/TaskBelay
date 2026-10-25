"""Run the Go-only entry point for independent candidate correctness assessment."""

from assess_candidate import assess as assess_candidate, main, test_status, working_files


def assess(task_directory, source_repository, workspace, agent_directory, output, runtime):
    return assess_candidate(task_directory, source_repository, workspace, agent_directory, output, runtime,
                            required_language="Go")


if __name__ == "__main__":
    main(required_language="Go")
