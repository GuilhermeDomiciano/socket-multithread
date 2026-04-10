import java.io.*;
import java.net.*;
import java.util.Random;
import java.util.concurrent.atomic.AtomicInteger;

public class TesteCarga {

    static final String HOST = "127.0.0.1"; // Troque pelo IP do servidor se for outra máquina
    static final int PORT = 5000;
    static final int NUM_CLIENTES = 5000;
    static final int OPERACOES_POR_CLIENTE = 200;

    // AtomicInteger é thread-safe — não precisa de lock manual
    static AtomicInteger totalOk = new AtomicInteger(0);
    static AtomicInteger totalErro = new AtomicInteger(0);

    static String gerarOperacao() {
        Random rand = new Random();
        int a = rand.nextInt(999) + 1;
        int b = rand.nextInt(999) + 1;
        String[] ops = {"+", "-", "*", "/"};
        String op = ops[rand.nextInt(ops.length)];
        return a + " " + op + " " + b;
    }

    static void clienteSimulado(int id) {
        int ok = 0, erro = 0;
        try (Socket socket = new Socket(HOST, PORT);
             BufferedReader in = new BufferedReader(new InputStreamReader(socket.getInputStream()));
             PrintWriter out = new PrintWriter(socket.getOutputStream(), true)) {

            for (int i = 0; i < OPERACOES_POR_CLIENTE; i++) {
                String op = gerarOperacao();
                out.println(op);
                String resultado = in.readLine();
                if (resultado != null && resultado.startsWith("Erro")) {
                    erro++;
                } else {
                    ok++;
                }
            }
        } catch (IOException e) {
            System.out.println("[Cliente " + id + "] ERRO de conexão: " + e.getMessage());
            erro += OPERACOES_POR_CLIENTE;
        }

        totalOk.addAndGet(ok);
        totalErro.addAndGet(erro);
        System.out.printf("[Cliente %03d] Concluído — %d OK, %d erros%n", id, ok, erro);
    }

    public static void main(String[] args) throws InterruptedException {
        int totalReqs = NUM_CLIENTES * OPERACOES_POR_CLIENTE;

        System.out.println("=======================================================");
        System.out.println("  TESTE DE CARGA");
        System.out.println("  Servidor:       " + HOST + ":" + PORT);
        System.out.println("  Clientes:       " + NUM_CLIENTES);
        System.out.println("  Ops/cliente:    " + OPERACOES_POR_CLIENTE);
        System.out.println("  Total de reqs:  " + totalReqs);
        System.out.println("=======================================================");

        Thread[] threads = new Thread[NUM_CLIENTES];
        for (int i = 0; i < NUM_CLIENTES; i++) {
            final int id = i + 1;
            threads[i] = new Thread(() -> clienteSimulado(id));
        }

        long inicio = System.currentTimeMillis();

        // Dispara TODOS antes de qualquer join
        for (Thread t : threads) t.start();
        for (Thread t : threads) t.join();

        double duracao = (System.currentTimeMillis() - inicio) / 1000.0;

        System.out.println("\n=======================================================");
        System.out.println("  RESULTADO FINAL");
        System.out.println("=======================================================");
        System.out.println("  Requisições enviadas:  " + totalReqs);
        System.out.println("  Sucesso:               " + totalOk.get());
        System.out.println("  Erros:                 " + totalErro.get());
        System.out.printf( "  Tempo total:           %.2fs%n", duracao);
        System.out.printf( "  Requisições/segundo:   %.1f%n", totalReqs / duracao);
        System.out.println("=======================================================");
    }
}