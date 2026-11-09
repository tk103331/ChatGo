// 示例插件入口。清单声明贡献，这里只实现行为。
// 宿主 API 是同步的，但函数可以写成 async（返回的 Promise 会在返回前结算）。

export const tools = {
  forecast(args, ctx) {
    const city = String(args.city || "").trim() || "未知城市";
    const seed = [...city].reduce((sum, ch) => sum + ch.codePointAt(0), 0);
    const temps = [18, 21, 24, 27, 30];
    const sky = ["晴", "多云", "小雨"];
    ctx.log.info("forecast", city);
    return {
      city,
      tempC: temps[seed % temps.length],
      condition: sky[seed % sky.length],
      note: "示例数据，不访问真实天气服务",
    };
  },

  // 返回声明式视图：宿主用表格渲染，而不是展示一段 JSON。
  forecast_table(args) {
    const cities = Array.isArray(args.cities) && args.cities.length
      ? args.cities
      : ["北京", "上海", "广州"];
    const temps = [18, 21, 24, 27, 30];
    const sky = ["晴", "多云", "小雨"];
    const rows = cities.map((city) => {
      const seed = [...city].reduce((sum, ch) => sum + ch.codePointAt(0), 0);
      return [city, temps[seed % temps.length], sky[seed % sky.length]];
    });
    return {
      $chatgo: {
        v: 1,
        view: {
          type: "table",
          title: "示例天气",
          columns: [
            { key: "city", label: "城市" },
            { key: "temp", label: "温度(°C)" },
            { key: "sky", label: "天气" },
          ],
          rows,
        },
      },
    };
  },
};
