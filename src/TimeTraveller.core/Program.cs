
internal class Program
{
    static System.Timers.Timer _mainline;

    //
    private static void Main(string[] args)
    {
        Init();



        Grpc.Core.Server server = new Grpc.Core.Server();
        server.Start();
        Console.WriteLine("Grpc server started");


        Console.ReadKey();
        // Close();
        Console.WriteLine("Timer going to end");
    }

    /// <summary>
    /// Initialize
    /// </summary>
    private static void Init()
    {
        _mainline = new System.Timers.Timer();
        _mainline.Interval = 1 * 1000; //1s
        _mainline.Elapsed += _mainline_Elapsed;


    }

    /// <summary>
    ///  Main timeline
    /// </summary>
    /// <param name="sender"></param>
    /// <param name="e"></param>
    private static void _mainline_Elapsed(object? sender, System.Timers.ElapsedEventArgs e)
    {
        //
    }
}