using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using System.Text.Json;
using System.Text.RegularExpressions;
using System.Windows.Input;

namespace LumiTimeWpf;

public partial class MainWindowVM : ObservableObject
{
    [ObservableProperty]
    private string rawTimeSchedule;

    public ICommand OnSubmitNewSchedule => new RelayCommand(OnSaveSchedule);


    private void OnSaveSchedule()
    {
        var events = new List<ScheduleEvent>();

        // 正则表达式匹配每行事件
        var lines = rawTimeSchedule.Split(new[] { '\r', '\n' }, StringSplitOptions.RemoveEmptyEntries);
        foreach (var line in lines)
        {
            if (line.Trim().StartsWith("**") || line.Trim().StartsWith("----------------------------------------------------------------------"))
                continue;

            // 匹配带时间戳的行
            var timestampMatch = Regex.Match(line, @"<t:(\d+):[FR]>");
            var platformMatch = Regex.Match(line, @"<:Logo(\w+):\d+>");

            if (!timestampMatch.Success && !line.Contains("No idea what time"))
                continue;

            long? timestamp = null;
            if (timestampMatch.Success)
                timestamp = long.Parse(timestampMatch.Groups[1].Value);

            string platform = "Unknown";
            if (platformMatch.Success)
                platform = platformMatch.Groups[1].Value;

            // 提取标题
            var titleMatch = Regex.Match(line, @"\*\*(.+?)\*\*");
            string title = titleMatch.Success ? titleMatch.Groups[1].Value.Trim() : "未知活动";

            // 构建链接
            string link = "https://example.com";
            switch (platform)
            {
                case "YouTube":
                    link = "https://www.youtube.com/@KanekoLumi";
                    break;
                case "Twitch":
                    link = "https://www.twitch.tv/kanekolumi";
                    break;
                case "Discord":
                    link = "https://www.twitch.tv/kanekolumi"; // 可替换为 Discord 链接
                    break;
            }

            events.Add(new ScheduleEvent
            {
                timestamp = timestamp,
                platform = platform,
                title = title,
                link = link
            });
        }

        // 序列化为 JSON
        var options = new JsonSerializerOptions { WriteIndented = true };
        string json = JsonSerializer.Serialize(events, options);


    }
}

class ScheduleEvent
{
    public long? timestamp { get; set; }
    public string platform { get; set; }
    public string title { get; set; }
    public string link { get; set; }
}

