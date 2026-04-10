import java.io.*;
import java.net.*;

public class Servidor {

    static final int PORT = 5000;

    public static void main(String[] args) throws IOException {
        ServerSocket serverSocket = new ServerSocket(PORT);
        System.out.println("Servidor aguardando conexões na porta " + PORT + "...");

        while (true) {
            Socket clientSocket = serverSocket.accept();
            System.out.println("[+] Cliente conectado: " + clientSocket.getInetAddress());

            // Cria uma thread nova para cada cliente
            Thread t = new Thread(new ClienteHandler(clientSocket));
            t.start();
            System.out.println("[Threads ativas: " + Thread.activeCount() + "]");
        }
    }
}

class ClienteHandler implements Runnable {

    private Socket socket;

    public ClienteHandler(Socket socket) {
        this.socket = socket;
    }

    @Override
    public void run() {
        String threadName = Thread.currentThread().getName(); // Nome único da thread
        System.out.println("[" + threadName + "] Atendendo cliente: " + socket.getInetAddress());

        try (
            BufferedReader in = new BufferedReader(new InputStreamReader(socket.getInputStream()));
            PrintWriter out = new PrintWriter(socket.getOutputStream(), true)
        ) {
            String linha;
            while ((linha = in.readLine()) != null) {
                System.out.println("[" + threadName + "] Recebeu: " + linha);
                String resultado = calcular(linha.trim());
                out.println(resultado);
                System.out.println("[" + threadName + "] Enviou: " + resultado);
            }
        } catch (IOException e) {
            System.out.println("[" + threadName + "] Cliente desconectado.");
        }
    }

    // Avalia operações básicas: +  -  *  /
    private String calcular(String expressao) {
        try {
            // Valida que só tem caracteres permitidos
            if (!expressao.matches("[0-9+\\-*/.() ]+")) {
                return "Erro: caracteres inválidos";
            }
            double resultado = avaliar(expressao.replaceAll("\\s+", ""));
            // Se o resultado for inteiro, exibe sem casas decimais
            if (resultado == Math.floor(resultado)) {
                return String.valueOf((long) resultado);
            }
            return String.valueOf(resultado);
        } catch (Exception e) {
            return "Erro: " + e.getMessage();
        }
    }

    // Parser simples de expressões matemáticas (sem eval — Java não tem)
    private double avaliar(String expr) {
        return new Object() {
            int pos = 0;

            double parse() {
                double result = parseTermo();
                while (pos < expr.length()) {
                    if (expr.charAt(pos) == '+') { pos++; result += parseTermo(); }
                    else if (expr.charAt(pos) == '-') { pos++; result -= parseTermo(); }
                    else break;
                }
                return result;
            }

            double parseTermo() {
                double result = parseFator();
                while (pos < expr.length()) {
                    if (expr.charAt(pos) == '*') { pos++; result *= parseFator(); }
                    else if (expr.charAt(pos) == '/') {
                        pos++;
                        double divisor = parseFator();
                        if (divisor == 0) throw new ArithmeticException("divisão por zero");
                        result /= divisor;
                    }
                    else break;
                }
                return result;
            }

            double parseFator() {
                if (expr.charAt(pos) == '(') {
                    pos++; // consome '('
                    double result = parse();
                    pos++; // consome ')'
                    return result;
                }
                int start = pos;
                if (expr.charAt(pos) == '-') pos++;
                while (pos < expr.length() && (Character.isDigit(expr.charAt(pos)) || expr.charAt(pos) == '.')) {
                    pos++;
                }
                return Double.parseDouble(expr.substring(start, pos));
            }
        }.parse();
    }
}