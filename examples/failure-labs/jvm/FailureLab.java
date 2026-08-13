import java.io.IOException;

final class FailureLab {
    private FailureLab() {
    }

    private static void readQueue() throws IOException {
        throw new IOException("queue storage returned a truncated frame");
    }

    private static void runWorker() {
        try {
            readQueue();
        } catch (IOException error) {
            throw new IllegalStateException("queue consumer cannot start", error);
        }
    }

    public static void main(String[] arguments) {
        runWorker();
    }
}
