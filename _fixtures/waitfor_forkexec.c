#include <errno.h>
#include <signal.h>
#include <stdio.h>
#include <string.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

/*
 * This fixture makes the fork-to-exec transition observable to wait-for tests.
 * Its expected invocation is:
 *
 *     waitfor_forkexec --launch EXECUTABLE --target [UNIQUE_ARG]
 *
 * In launcher mode it forks a child that stops itself before exec. The parent
 * waits until the child is stopped, reports its PID, and waits for it to exit.
 * The test can inspect the stopped child with its inherited, pre-exec command
 * line and then send SIGCONT, causing the child to exec into target mode.
 *
 * In target mode the fixture remains alive so the test can find and terminate
 * it. The alarm prevents an orphaned fixture from surviving indefinitely if
 * the test process is killed before cleanup.
 */
int main(int argc, char **argv) {
	if (argc >= 2 && strcmp(argv[1], "--target") == 0) {
		alarm(30);
		for (;;)
			pause();
	}
	if (argc < 4 || strcmp(argv[1], "--launch") != 0 ||
	    strcmp(argv[3], "--target") != 0)
		return 2;

	pid_t pid = fork();
	if (pid < 0)
		return 3;
	if (pid == 0) {
		/* Stop before exec; the test resumes us with SIGCONT after scanning. */
		raise(SIGSTOP);
		execv(argv[2], &argv[2]);
		return errno;
	}

	int status;
	/*
	 * Wait until the child has stopped itself. WUNTRACED lets waitpid return
	 * for a stopped child; WIFSTOPPED verifies that it stopped rather than exited.
	 */
	if (waitpid(pid, &status, WUNTRACED) != pid || !WIFSTOPPED(status))
		return 4;
	printf("%d\n", pid);
	fflush(stdout);

	/* The test eventually terminates the target; wait here to reap it. */
	if (waitpid(pid, &status, 0) != pid)
		return 5;
	return 0;
}
