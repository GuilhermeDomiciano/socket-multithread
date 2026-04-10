import java.io.*;
import java.net.*;
import java.util.Scanner;

public class Cliente {

    static final String HOST = "127.0.0.1";
    static final int PORT = 5000;

    public static void main(String[] args) throws IOException {
        Socket socket = new Socket(HOST, PORT);
        System.out.println("Conectado ao servidor " + HOST + ":" + PORT);
        System.out.println("Digite operações matemáticas (ex: 2 + 3, 10 / 2). Ou 'sair' para encerrar.\n");

        BufferedReader in = new BufferedReader(new InputStreamReader(socket.getInputStream()));
        PrintWriter out = new PrintWriter(socket.getOutputStream(), true);
        Scanner scanner = new Scanner(System.in);

        while (true) {
            System.out.print("Operação: ");
            String linha = scanner.nextLine().trim();
            if (linha.equalsIgnoreCase("sair")) break;
            out.println(linha);
            System.out.println("Resultado: " + in.readLine() + "\n");
        }

        socket.close();
    }
}