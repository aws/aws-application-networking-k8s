# GRPCRoute Support

## What is `GRPCRoute`?

The `GRPCRoute` is a custom resource defined in the Gateway API that specifies how gRPC traffic should be routed.
It allows you to set up routing rules based on various match criteria, such as service names and methods.
With `GRPCRoute`, you can ensure that your gRPC traffic is directed to the appropriate backend services in a
Kubernetes environment.

For a detailed reference on `GRPCRoute` from the Gateway API, please check the official
[Gateway API documentation](https://gateway-api.sigs.k8s.io/references/spec/#networking.x-k8s.io/v1alpha2.GRPCRoute).

## Setting up a HelloWorld gRPC Server

In this section, we'll walk you through deploying a simple "HelloWorld" gRPC server and setting up the required
routing rules using the Gateway API.

**Deploying the Necessary Resources**

1. *Apply the Gateway Configuration*: This YAML file contains the definition for a gateway with an HTTPS listener.
   ```bash
   kubectl apply -f files/examples/my-hotel-gateway-multi-listeners.yaml
   ```

2. *Deploy the gRPC Server*: Deploy the example gRPC server which will respond to the SayHello gRPC request.
   ```bash
   kubectl apply -f files/examples/greeter-grpc-server.yaml
   ```

3. *Set Up the gRPC Route*:This YAML file contains the `GRPCRoute` resource which directs the gRPC traffic to our example server.
   ```bash
   kubectl apply -f files/examples/greeter-grpc-route.yaml
   ```

4. *Verify the Deployment*:Check to make sure that our gRPC server pod is running and get its name.
   ```bash
   kubectl get pods -A
   ```

**Testing the gRPC Server**

1. *Access the gRPC Server Pod*: Copy the name of the pod running the `greeter-grpc-server` and use it to access the pod's shell.
   ```bash
   kubectl exec -it <name-of-grpc-server-pod> -- bash
   ```

2. *Prepare the Test Client*: Inside the pod shell, create a test client by pasting the provided Go code.
   ```bash
   cat << EOF > test.go
   package main
   
   import (
      "crypto/tls"
      "log"
      "os"
   
      "golang.org/x/net/context"
      "google.golang.org/grpc"
      "google.golang.org/grpc/credentials"
      pb "google.golang.org/grpc/examples/helloworld/helloworld"
   )
   
   func main() {
      if len(os.Args) < 3 {
      log.Fatalf("Usage: %s <address> <port>", os.Args[0])
      }
   
      address := os.Args[1] + ":" + os.Args[2]

      // Create a connection with insecure TLS (no server verification).
      creds := credentials.NewTLS(&tls.Config{
          InsecureSkipVerify: true,
      })
      conn, err := grpc.Dial(address, grpc.WithTransportCredentials(creds))
      if err != nil {
          log.Fatalf("did not connect: %v", err)
      }
      defer conn.Close()
      c := pb.NewGreeterClient(conn)
   
      // Contact the server and print out its response.
      name := "world"
      if len(os.Args) > 3 {
          name = os.Args[3]
      }
      r, err := c.SayHello(context.Background(), &pb.HelloRequest{Name: name})
      if err != nil {
          log.Fatalf("could not greet: %v", err)
      }
      log.Printf("Greeting: %s", r.Message)
   }
   EOF
   ```

3. *Run the Test Client*: Execute the test client, making sure to replace `<SERVICE DNS>` with the VPC Lattice service DNS and `<PORT>`
   with the port your Lattice listener uses (in this example, we use 443).
   ```bash
   go run test.go <SERVICE DNS> <PORT>
   ```

**Expected Output**

If everything is set up correctly, you should see the following output:

```sh
Greeting: Hello world
```

This confirms that our gRPC request was successfully routed through VPC Lattice and processed by our `greeter-grpc-server`.
